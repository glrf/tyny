package store

import "errors"

var NotFound = errors.New("not found")
var Conflict = errors.New("conflict")
