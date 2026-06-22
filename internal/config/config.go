package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
	"github.com/fsnotify/fsnotify"
)

type Config struct {
	User      UserConfig      `yaml:"user"`
	Schedule  ScheduleConfig  `yaml:"schedule"`
	Weekly    WeeklyConfig    `yaml:"weekly"`
	Interests []InterestConfig `yaml:"interests"`
	Search    SearchConfig    `yaml:"search"`
	Models    ModelsConfig    `yaml:"models"`
	Budgets   BudgetsConfig   `yaml:"budgets"`
	Email     EmailConfig     `yaml:"email"`
	Paths     PathsConfig     `yaml:"paths"`
}

type UserConfig struct {
	Email string `yaml:"email"`
	Name  string `yaml:"name"`
}

type ScheduleConfig struct {
	Cron     string `yaml:"cron"`
	Timezone string `yaml:"timezone"`
	CatchUp  bool   `yaml:"catch_up"`
}

type WeeklyConfig struct {
	MaxItems     int `yaml:"max_items"`
	PrimaryFloor int `yaml:"primary_floor"`
}

type InterestConfig struct {
	Tag       string   `yaml:"tag"`
	Primary   bool     `yaml:"primary"`
	Intensity string   `yaml:"intensity"`
	Types     []string `yaml:"types"`
}

type SearchConfig struct {
	Priority       []string       `yaml:"priority"`
	Fanout         bool           `yaml:"fanout"`
	MonthlyCaps    map[string]int `yaml:"monthly_caps"`
	SearxngBaseURL string         `yaml:"searxng_base_url"`
}

type ModelsConfig struct {
	Triage   string `yaml:"triage"`
	Research string `yaml:"research"`
}

type BudgetsConfig struct {
	ResearchMaxRounds    int   `yaml:"research_max_rounds"`
	ResearchMaxToolCalls int   `yaml:"research_max_tool_calls"`
	PerRunTokenCap       int64 `yaml:"per_run_token_cap"`
}

type EmailConfig struct {
	Channel     string `yaml:"channel"`
	AttachMaxMB int    `yaml:"attach_max_mb"`
}

type PathsConfig struct {
	DataDir string `yaml:"data_dir"`
}

type Loader struct {
	path string
	mu   sync.RWMutex
	cfg  *Config

	watcher *fsnotify.Watcher
	OnReload func(*Config, error)
}

func NewLoader(path string) *Loader {
	return &Loader{
		path: path,
	}
}

func (l *Loader) Load() (*Config, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validate(&c); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	l.mu.Lock()
	l.cfg = &c
	l.mu.Unlock()

	return &c, nil
}

func (l *Loader) Get() *Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cfg
}

func (l *Loader) Watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	l.watcher = watcher

	// Watch the directory instead of the file directly to handle atomic saves (some editors replace the file)
	dir := filepath.Dir(l.path)
	if err := l.watcher.Add(dir); err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-l.watcher.Events:
				if !ok {
					return
				}
				// Check if the modified file is our config file
				if filepath.Clean(event.Name) == filepath.Clean(l.path) {
					if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
						c, err := l.Load()
						if l.OnReload != nil {
							l.OnReload(c, err)
						}
					}
				}
			case err, ok := <-l.watcher.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "config watcher error: %v\n", err)
			}
		}
	}()

	return nil
}

func (l *Loader) Close() error {
	if l.watcher != nil {
		return l.watcher.Close()
	}
	return nil
}

func validate(c *Config) error {
	if c.User.Email == "" {
		return fmt.Errorf("user.email is required")
	}
	
	primaryCount := 0
	for _, intConfig := range c.Interests {
		if intConfig.Primary {
			primaryCount++
		}
		if intConfig.Intensity != "high" && intConfig.Intensity != "medium" && intConfig.Intensity != "light" {
			return fmt.Errorf("interest %q has invalid intensity %q", intConfig.Tag, intConfig.Intensity)
		}
	}
	
	if primaryCount != 1 {
		return fmt.Errorf("exactly one interest must be marked as primary, found %d", primaryCount)
	}
	
	if c.Weekly.MaxItems <= 0 {
		return fmt.Errorf("weekly.max_items must be > 0")
	}

	return nil
}
