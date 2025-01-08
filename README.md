### Example usage

```go
package main

import (
	"fmt"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	"github.com/graph-gophers/graphql-transport-ws/graphqlws"
)

const schema = `
	schema {
		subscription: Subscription
	}

	type Subscription {
		...
	}
`

type resolver struct {
	// ...
}

func main() {
	// init graphQL schema
	s, err := graphql.ParseSchema(schema, &resolver{})
	if err != nil {
		panic(err)
	}

	// graphQL handler
	graphQLHandler := graphqlws.NewHandlerFunc(s, &relay.Handler{Schema: s})
	http.HandleFunc("/graphql", graphQLHandler)

	// start HTTP server
	if err := http.ListenAndServe(fmt.Sprintf(":%d", 8080), nil); err != nil {
		panic(err)
	}
}
```
