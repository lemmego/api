package res

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/romsar/gonertia"
	"log"
	"os"
	"path"
)

const InertiaRootTemplatePath = "resources/views/root.html"
const InertiaManifestPath = "./public/build/manifest.json"
const InertiaBuildPath = "/public/build/"

type InertiaFlashProvider struct {
	errors map[string]gonertia.ValidationErrors
}

func NewInertiaFlashProvider() *InertiaFlashProvider {
	return &InertiaFlashProvider{errors: make(map[string]gonertia.ValidationErrors)}
}

func (p *InertiaFlashProvider) FlashErrors(ctx context.Context, errors gonertia.ValidationErrors) error {
	if sessionID, ok := ctx.Value("sessionID").(string); ok {
		p.errors[sessionID] = errors
	}
	return nil
}

func (p *InertiaFlashProvider) GetErrors(ctx context.Context) (gonertia.ValidationErrors, error) {
	var inertiaErrors gonertia.ValidationErrors
	if sessionID, ok := ctx.Value("sessionID").(string); ok {
		inertiaErrors = p.errors[sessionID]
		p.errors[sessionID] = nil
	}
	return inertiaErrors, nil
}

func NewInertia(rootTemplatePath string, opts ...gonertia.Option) *gonertia.Inertia {
	i, err := gonertia.NewFromFile(
		rootTemplatePath,
		opts...,
	)

	if err != nil {
		log.Fatal(err)
	}

	return i
}

func Vite(manifestPath, buildDir string) func(path string) (string, error) {
	f, err := os.Open(manifestPath)
	if err != nil {
		log.Fatalf("cannot open provided vite manifest file: %s", err)
	}
	defer f.Close()

	viteAssets := make(map[string]*struct {
		File   string `json:"file"`
		Source string `json:"src"`
	})
	err = json.NewDecoder(f).Decode(&viteAssets)
	if err != nil {
		log.Fatalf("cannot unmarshal vite manifest file to json: %s", err)
	}

	return func(p string) (string, error) {
		if val, ok := viteAssets[p]; ok {
			return path.Join(buildDir, val.File), nil
		}
		return "", fmt.Errorf("asset %q not found", p)
	}
}
