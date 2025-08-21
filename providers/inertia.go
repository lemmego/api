package providers

func init() {
	//app.RegisterService(func(a app.App) error {
	//	i := res.NewInertia(
	//		res.InertiaRootTemplatePath,
	//		gonertia.WithVersionFromFile(res.InertiaManifestPath),
	//		gonertia.WithSSR(),
	//		//inertia.WithVersion("1.0"),
	//		gonertia.WithFlashProvider(res.NewInertiaFlashProvider()),
	//	)
	//
	//	_, err := os.Stat(res.ViteHotPath)
	//	if err == nil {
	//		i.ShareTemplateFunc("vite", func(entry string) (string, error) {
	//			content, err := os.ReadFile(res.ViteHotPath)
	//			if err != nil {
	//				return "", err
	//			}
	//			url := strings.TrimSpace(string(content))
	//			if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
	//				url = url[strings.Index(url, ":")+1:]
	//			} else {
	//				url = "//localhost:5173"
	//			}
	//			if entry != "" && !strings.HasPrefix(entry, "/") {
	//				entry = "/" + entry
	//			}
	//			return url + entry, nil
	//		})
	//	} else {
	//		i.ShareTemplateFunc("vite", res.Vite(res.InertiaManifestPath, res.InertiaBuildPath))
	//	}
	//
	//	i.ShareTemplateData("env", a.Config().Get("app.env"))
	//
	//	return di.For[*gonertia.Inertia](a.Container()).
	//		AsSingleton().
	//		UseInstance(i)
	//})
}
