package app

type Provider interface {
	Register(a AppManager)
	Boot(a AppManager)
}

type ServiceProvider struct {
	App AppManager

	routes       []*Route
	publishables []*Publishable
}

//func (p *ServiceProvider) Publishes(filePath string, content []byte) {
//	if err := os.WriteFile(filePath, content, 0644); err != nil {
//		panic(err)
//	}
//}

func (p *ServiceProvider) Publishes(filePath string, content []byte, tag string) {
	p.publishables = append(p.publishables, &Publishable{
		FilePath: filePath,
		Content:  content,
		Tag:      tag,
	})
}

func (p *ServiceProvider) Publishables() []*Publishable {
	return p.publishables
}

func (p *ServiceProvider) AddRoutes(routeRegistrar func(r Router) []*Route) {
	p.routes = routeRegistrar(p.App.Router())
}

func (p *ServiceProvider) PublishMigration() []byte {
	return []byte{}
}

func (p *ServiceProvider) PublishConfig() []byte {
	return []byte{}
}
