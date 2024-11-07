// Copyright (c) 2020 Mercari, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package internal

import (
	"os"

	"github.com/gedex/inflector"
	"github.com/jinzhu/inflection"
	"gopkg.in/yaml.v2"
)

type Inflector interface {
	Singularize(string) string
	Pluralize(string) string
}

type DefaultInflector struct{}
type RuleInflector struct{}

func (i *DefaultInflector) Singularize(s string) string {
	return inflector.Singularize(s)
}
func (i *DefaultInflector) Pluralize(s string) string {
	return inflector.Pluralize(s)
}

func (i *RuleInflector) Singularize(s string) string {
	return inflection.Singular(s)
}
func (i *RuleInflector) Pluralize(s string) string {
	return inflection.Plural(s)
}

func NewInflector(ruleFile string) (Inflector, error) {
	if ruleFile == "" {
		return &DefaultInflector{}, nil
	}
	err := registerInflectorRule(ruleFile)
	if err != nil {
		return nil, err
	}
	return &RuleInflector{}, nil
}

type InflectRule struct {
	Singuler string `yaml:"singular"`
	Plural   string `yaml:"plural"`
}

func registerInflectorRule(inflectionRuleFile string) error {
	rules, err := readInflectorRule(inflectionRuleFile)
	if err != nil {
		return err
	}

	for _, irr := range rules {
		inflection.AddIrregular(irr.Singuler, irr.Plural)
	}
	return nil
}

func readInflectorRule(ruleFile string) ([]InflectRule, error) {
	data, err := os.ReadFile(ruleFile)
	if err != nil {
		return nil, err
	}
	var rules []InflectRule
	err = yaml.Unmarshal(data, &rules)
	if err != nil {
		return nil, err
	}
	return rules, nil

}
