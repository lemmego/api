package app

import "github.com/ggicci/httpin"

type BaseInput struct {
	*validator
	app App
	ctx Context
}

type FileInput struct {
	*httpin.File
}

func (bi *BaseInput) App() AppCore {
	return bi.app
}

func (bi *BaseInput) Ctx() Context {
	return bi.ctx
}

func (bi *BaseInput) Check() error {
	return bi.Validate()
}
