package app

type Provider interface {
	Register(a AppManager)
	Boot(a AppManager)
}

type Publisher interface {
	Publishes(publishables map[string][]byte, tag string)
	Publishables() []*Publishable
	RouteRegistrar
	CommandRegistrar
}

type RouteRegistrar interface {
	RouteCallback() func(r Router)
}

type CommandRegistrar interface {
	Commands() []Command
}

type ServiceProvider struct {
	App AppManager

	routeCallback func(r Router)
	commands      []Command
	publishables  []*Publishable
}

func (p *ServiceProvider) Register(a AppManager) {
	//TODO implement me
}

func (p *ServiceProvider) Boot(a AppManager) {
	//TODO implement me
}

func (p *ServiceProvider) Publishes(publishables map[string][]byte, tag string) {
	p.publishables = []*Publishable{}

	for filePath, content := range publishables {
		p.publishables = append(p.publishables, &Publishable{
			FilePath: filePath,
			Content:  content,
			Tag:      tag,
		})
	}
}

func (p *ServiceProvider) Publishables() []*Publishable {
	return p.publishables
}

func (p *ServiceProvider) AddRoutes(routeCallback func(r Router)) {
	p.routeCallback = routeCallback
}

func (p *ServiceProvider) AddCommands(commands []Command) {
	p.commands = append(p.commands, commands...)
}

func (p *ServiceProvider) RouteCallback() func(r Router) {
	return p.routeCallback
}

func (p *ServiceProvider) Commands() []Command {
	return p.commands
}
