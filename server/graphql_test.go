// handlers_test.go
package server

import (
	"net/http"
	"net/http/httptest"

	opentracing "github.com/opentracing/opentracing-go"
	observe "github.com/shah/observe-go"
	"github.com/stretchr/testify/suite"
)

type GraphQLOverHTTPServerSuite struct {
	suite.Suite
	observatory observe.Observatory
	span        opentracing.Span
}

func (suite *GraphQLOverHTTPServerSuite) SetupSuite() {
	observatory := observe.MakeObservatoryFromEnv()
	suite.observatory = observatory
	suite.span = observatory.StartTrace("ResourceSuite")
}

func (suite *GraphQLOverHTTPServerSuite) TearDownSuite() {
	suite.span.Finish()
	suite.observatory.Close()
}

func (suite *GraphQLOverHTTPServerSuite) TestHealthCheckHandler() {
	// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
	// pass 'nil' as the third parameter.
	req, err := http.NewRequest("GET", "/health-check", nil)
	suite.Nil(err, "Unable to create Request")

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(healthCheckHandler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	suite.Equal(rr.Code, http.StatusOK, "Invalid HTTP Status")

	// Check the response body is what we expect.
	expected := `{"alive": true}`
	suite.Equal(rr.Body.String(), expected, "Unexpected response")
}
