package internal

import (
	"strings"

	"github.com/kenshaw/snaker"
)

func NewSnaker(customWords []string) (*snaker.Initialisms, error) {
	defaultInitialisms := snaker.NewDefaultInitialisms()
	if customWords == nil {
		return defaultInitialisms, nil
	}

	pairs := make([]string, len(customWords)*2)
	for i := range customWords {
		pairs[i*2] = strings.ToUpper(customWords[i])
		pairs[i*2+1] = strings.ToUpper(customWords[i])
	}

	err := defaultInitialisms.Post(pairs...)
	if err != nil {
		return nil, err
	}

	return defaultInitialisms, nil
}
