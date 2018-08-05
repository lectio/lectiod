package server

import (
	"context"
	"io"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/handler"
	"github.com/lectio/lectiod/resolvers"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	observe "github.com/shah/observe-go"
)

func createGraphQLObservableResolverMiddleware(o observe.Observatory) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		rctx := graphql.GetResolverContext(ctx)
		span, ctx := o.StartTraceFromContext(ctx, rctx.Object+" Handler Middleware",
			opentracing.Tag{Key: "resolver.object", Value: rctx.Object},
			opentracing.Tag{Key: "resolver.field", Value: rctx.Field.Name},
		)
		defer span.Finish()
		ext.SpanKind.Set(span, "server")
		ext.Component.Set(span, "gqlgen")
		res, err := next(ctx)
		if err != nil {
			ext.Error.Set(span, true)
			span.LogFields(
				otlog.String("event", "error"),
				otlog.String("message", err.Error()),
				otlog.Error(err),
			)
		}

		return res, err
	}
}

func createGraphQLObservableRequestMiddleware(o observe.Observatory) graphql.RequestMiddleware {
	return func(ctx context.Context, next func(ctx context.Context) []byte) []byte {
		requestContext := graphql.GetRequestContext(ctx)
		span, ctx := o.StartTraceFromContext(ctx, "HTTP Request")
		defer span.Finish()
		span.LogFields(otlog.String("rawQuery", requestContext.RawQuery))
		// TODO ext.HTTPMethod.Set(span, ...)
		// TODO ext.HTTPUrl.Set(span, ...)
		ext.SpanKind.Set(span, "server")
		ext.Component.Set(span, "gqlgen")
		res := next(ctx)
		return res
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	// A very simple health check.
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	// In the future we could report back on the status of our DB, or our cache
	// (e.g. Redis) by performing a simple PING, and include them in the response.
	io.WriteString(w, `{"alive": true}`)
}

func createExecutableSchemaHandler(o observe.Observatory, provider resolvers.ConfigPathProvider, parent opentracing.Span) http.HandlerFunc {
	span := o.StartChildTrace("graphql.createExecutableSchemaHandler", parent)
	defer span.Finish()

	var cfg resolvers.Config
	cfg.Resolvers = resolvers.NewSchemaResolvers(o, provider, span)

	// TODO Add error presenter and panic handlers: https://gqlgen.com/reference/errors/

	return handler.GraphQL(resolvers.NewExecutableSchema(cfg),
		handler.ResolverMiddleware(createGraphQLObservableResolverMiddleware(o)),
		handler.RequestMiddleware(createGraphQLObservableRequestMiddleware(o)))
}

// CreateGraphQLOverHTTPServer prepares an HTTP server to run GraphQL queries
func CreateGraphQLOverHTTPServer(o observe.Observatory, provider resolvers.ConfigPathProvider, parent opentracing.Span) *http.Server {
	span := o.StartChildTrace("graphql.CreateGraphQLOverHTTPServer", parent)
	defer span.Finish()

	// TODO Add Voyager documentation handler: https://github.com/APIs-guru/graphql-voyager

	serveMux := http.NewServeMux()
	serveMux.Handle("/", handler.Playground("Lectio", "/graphql"))
	serveMux.Handle("/graphql", createExecutableSchemaHandler(o, provider, span))
	serveMux.HandleFunc("/health-check", healthCheckHandler)

	server := http.Server{
		Addr:    ":8080",
		Handler: serveMux,
	}
	return &server
}
