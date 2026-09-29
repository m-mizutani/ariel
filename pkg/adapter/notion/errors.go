package notion

import (
	"encoding/json"
	"net/http"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
)

// errorBody is Notion's error response. The message is not attached to
// errors: it can quote request content.
type errorBody struct {
	Code string `json:"code"`
}

// wrapError maps a Notion error response to the interfaces errors, using the
// status code and the error code of Notion's "Status codes" reference.
func wrapError(status int, body []byte, msg string) error {
	var eb errorBody
	_ = json.Unmarshal(body, &eb) // a body that is not JSON leaves the code empty
	vals := []goerr.Option{goerr.V("notion_status", status), goerr.V("notion_code", eb.Code)}

	switch {
	// invalid_client is also 401, but it rejects Ariel's client credentials,
	// not the user's token.
	case status == http.StatusUnauthorized && eb.Code != "invalid_client",
		status == http.StatusBadRequest && eb.Code == "invalid_grant":
		return goerr.Wrap(interfaces.ErrNotionTokenInvalid, msg, vals...)
	case status == http.StatusForbidden:
		return goerr.Wrap(interfaces.ErrNotionForbidden, msg, vals...)
	case status == http.StatusNotFound:
		return goerr.Wrap(interfaces.ErrNotionNotFound, msg, vals...)
	case status == http.StatusTooManyRequests:
		return goerr.Wrap(interfaces.ErrNotionRateLimited, msg, vals...)
	default:
		return goerr.New(msg, vals...)
	}
}
