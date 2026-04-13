package server

import (
	"fmt"
	"net/url"
)

// All URL patterns are defined here. Handlers call route methods to generate
// URLs — no URL strings are hardcoded elsewhere. Type names and record keys
// are PathEscape'd so that `/`, `?`, `#`, and similar reserved characters
// produce correctly-encoded URL segments.

const (
	patternHome     = "/"
	patternTypeList = "/types/{TypeName}"
	patternRecord   = "/types/{TypeName}/{Key}"
)

// homeRoute represents GET / and HEAD /.
type homeRoute struct{}

func (homeRoute) URL() string { return "/" }

// typeRoute represents GET /types/{TypeName}.
type typeRoute struct{ TypeName string }

func (r typeRoute) URL() string {
	return fmt.Sprintf("/types/%s", url.PathEscape(r.TypeName))
}

// recordRoute represents GET /types/{TypeName}/{Key}.
type recordRoute struct {
	TypeName string
	Key      string
}

func (r recordRoute) URL() string {
	return fmt.Sprintf("/types/%s/%s", url.PathEscape(r.TypeName), url.PathEscape(r.Key))
}
