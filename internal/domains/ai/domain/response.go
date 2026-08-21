package domain

import "errors"

var ErrInvalidRequest = errors.New("invalid Responses request")

type ProxyResponse struct {
	Status      int
	ContentType string
	Body        []byte
}
