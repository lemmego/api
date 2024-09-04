package app

import "github.com/ggicci/httpin"

type BaseInput struct {
	App       AppManager
	Ctx       *Context
	Validator *Validator
}

type FileInput struct {
	*httpin.File
}
