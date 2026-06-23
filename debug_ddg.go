package main

import (
	"context"
	"fmt"
	"dailyread/internal/search"
	"dailyread/internal/search/ddg"
)

func main() {
	p := ddg.New()
	res, err := p.Search(context.Background(), search.Query{Text: "Raft consensus algorithm", MaxResults: 5})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println(res[0].Content)
}
