package app

import (
	"github.com/lemmego/api/res"
	"github.com/romsar/gonertia"
)

type InertiaProvider struct {
	*ServiceProvider
}

func (provider *InertiaProvider) Register(a AppManager) {
	i := res.NewInertia(
		res.InertiaRootTemplatePath,
		gonertia.WithVersionFromFile(res.InertiaManifestPath),
		gonertia.WithSSR(),
		//inertia.WithVersion("1.0"),
		gonertia.WithFlashProvider(res.NewInertiaFlashProvider()),
	)

	i.ShareTemplateFunc("vite", res.Vite(res.InertiaManifestPath, res.InertiaBuildPath))
	i.ShareTemplateData("env", a.Config().Get("app.env"))

	provider.App.AddService(i)
}

func (provider *InertiaProvider) Boot(a AppManager) {
	//
}
