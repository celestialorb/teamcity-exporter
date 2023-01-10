package main

import (
	"strings"
	"time"
)

type TeamCityTime struct {
	time.Time
}

func (t *TeamCityTime) UnmarshalJSON(b []byte) error {
	text := strings.Trim(string(b), "\"")
	tm, err := time.Parse("20060102T150405-0700", text)
	t.Time = tm
	return err
}
