package app

import "github.com/ggicci/httpin"

type BaseInput struct {
	*Validator
	app App
	ctx *ctx
}

type FileInput struct {
	*httpin.File
}

func (bi *BaseInput) App() AppCore {
	return bi.app
}

func (bi *BaseInput) Ctx() *ctx {
	return bi.ctx
}

func (bi *BaseInput) Check() error {
	return bi.Validate()
}
