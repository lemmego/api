package providers

import (
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/res"
	"github.com/romsar/gonertia"
)

func init() {
	app.RegisterService(func(a app.App) error {
		i := res.NewInertia(
			res.InertiaRootTemplatePath,
			gonertia.WithVersionFromFile(res.InertiaManifestPath),
			gonertia.WithSSR(),
			//inertia.WithVersion("1.0"),
			gonertia.WithFlashProvider(res.NewInertiaFlashProvider()),
		)

		i.ShareTemplateFunc("vite", res.Vite(res.InertiaManifestPath, res.InertiaBuildPath))
		i.ShareTemplateData("env", a.Config().Get("app.env"))

		a.AddService(i)
		return nil
	})
}
