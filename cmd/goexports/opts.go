package main

import (
	"flag"
	"regexp"
)

type filterFlag struct {
	ops []filterOp
}

type filterFlagValue struct {
	ignore bool
	f      *filterFlag
}

type filterOp struct {
	ignore bool
	r      *regexp.Regexp
}

func (f *filterFlag) ignore(name string) bool {
	for _, op := range f.ops {
		if op.r.MatchString(name) {
			return op.ignore
		}
	}
	return false
}

func (f *filterFlag) report(name string) bool {
	return !f.ignore(name)
}

func (v *filterFlagValue) String() string {
	return "<filter>"
}

func (v *filterFlagValue) Set(arg string) error {
	r, err := regexp.Compile(arg)
	if err != nil {
		return err
	}

	v.f.ops = append(v.f.ops, filterOp{ignore: v.ignore, r: r})
	return nil
}

func registerFilterFlag(include, exclude, description string) *filterFlag {
	f := &filterFlag{}
	incVal := &filterFlagValue{false, f}
	excVal := &filterFlagValue{true, f}
	flag.Var(incVal, include, "include "+description)
	flag.Var(excVal, exclude, "exclude "+description)
	return f
}
