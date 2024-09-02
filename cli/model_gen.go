package cli

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/lemmego/api/fsys"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

//go:embed model.txt
var modelStub string

var modelFieldTypes = []string{
	"int", "uint", "int64", "uint64", "float64", "string", "bool", "time.Time", "custom",
}

const (
	TagColumn                 = "column"
	TagType                   = "type"
	TagSerializer             = "serializer"
	TagSize                   = "size"
	TagPrimaryKey             = "primaryKey"
	TagUnique                 = "unique"
	TagDefault                = "default"
	TagPrecision              = "precision"
	TagScale                  = "scale"
	TagNotNull                = "not null"
	TagAutoIncrement          = "autoIncrement"
	TagAutoIncrementIncrement = "autoIncrementIncrement"
	TagEmbedded               = "embedded"
	TagEmbeddedPrefix         = "embeddedPrefix"
	TagAutoCreateTime         = "autoCreateTime"
	TagAutoUpdateTime         = "autoUpdateTime"
	TagIndex                  = "index"
	TagUniqueIndex            = "uniqueIndex"
	TagCheck                  = "check"
	TagWritePerm              = "<-"
	TagReadPerm               = "->"
	TagIgnore                 = "-"
	TagComment                = "comment"
)

type ModelField struct {
	Name               string
	Type               string
	Required           bool
	Unique             bool
	Primary            bool
	ForeignConstrained bool
}

type ModelConfig struct {
	Name   string
	Fields []*ModelField
}

type ModelGenerator struct {
	name   string
	fields []*ModelField
}

type DBTag struct {
	Name     string
	Argument string
}

type DBTagBuilder struct {
	tags       []*DBTag
	driverName string
}

func NewDBTagBuilder(tags []*DBTag, driverName string) *DBTagBuilder {
	return &DBTagBuilder{tags, driverName}
}

func (mtb *DBTagBuilder) Add(name, argument string) *DBTagBuilder {
	mtb.tags = append(mtb.tags, &DBTag{name, argument})
	return mtb
}

func (mtb *DBTagBuilder) Build() string {
	// Build the tag string in this format: gorm:"tagName1:tagArgument1,tagName2:tagArgument2".
	// If the argument is empty, it's omitted: gorm:"tagName1,tagName2".
	var tagStrs []string
	for _, t := range mtb.tags {
		if t.Argument != "" {
			tagStrs = append(tagStrs, fmt.Sprintf(`%s:%s`, t.Name, t.Argument))
		} else {
			tagStrs = append(tagStrs, fmt.Sprintf(`%s`, t.Name))
		}
	}
	if len(tagStrs) == 0 {
		return ""
	}
	return fmt.Sprintf("%s:", mtb.driverName) + "\"" + strings.Join(tagStrs, ",") + "\""
}

func NewModelGenerator(mc *ModelConfig) *ModelGenerator {
	return &ModelGenerator{mc.Name, mc.Fields}
}

func (mg *ModelGenerator) GetPackagePath() string {
	return "internal/models"
}

func (mg *ModelGenerator) GetStub() string {
	return modelStub
}

func (mg *ModelGenerator) Generate() error {
	fs := fsys.NewLocalStorage("")
	parts := strings.Split(mg.GetPackagePath(), "/")
	packageName := mg.GetPackagePath()

	if len(parts) > 0 {
		packageName = parts[len(parts)-1]
	}

	tmplData := map[string]interface{}{
		"PackageName": packageName,
		"ModelName":   mg.name,
		"Fields":      mg.fields,
	}

	output, err := ParseTemplate(tmplData, mg.GetStub(), commonFuncs)

	if err != nil {
		return err
	}

	if exists, _ := fs.Exists(mg.GetPackagePath()); exists {
		err = fs.Write(mg.GetPackagePath()+"/"+mg.name+".go", []byte(output))

		if err != nil {
			return err
		}
	} else {
		fs.CreateDirectory(mg.GetPackagePath())
		err = fs.Write(mg.GetPackagePath()+"/"+mg.name+".go", []byte(output))

		if err != nil {
			return err
		}
	}

	return nil
}

func (mg *ModelGenerator) Command() *cobra.Command {
	return modelCmd
}

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Generate a db model",
	Long:  `Generate a db model`,
	Run: func(cmd *cobra.Command, args []string) {
		var modelName string
		var fields []*ModelField

		nameForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter the model name in snake_case and singular form").
					Value(&modelName).
					Validate(SnakeCase),
			),
		)
		err := nameForm.Run()
		if err != nil {
			return
		}

		for {
			var fieldName, fieldType string
			const required = "Required"
			const unique = "Unique"
			selectedAttrs := []string{}

			fieldNameForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Enter the field name in snake_case.\nThe following fields will be provided:\nid, created_at, updated_at, deleted_at").
						Validate(SnakeCaseEmptyAllowed).
						Validate(
							NotIn(
								[]string{"id", "created_at", "updated_at", "deleted_at"},
								"No need, this field will be provided",
							),
						).
						Value(&fieldName),
				),
			)
			err := fieldNameForm.Run()
			if err != nil {
				return
			}
			if fieldName == "" {
				break
			}

			fieldTypeForm := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("What should the data type be?").
						Options(huh.NewOptions(modelFieldTypes...)...).
						Value(&fieldType),
				),
			)
			err = fieldTypeForm.Run()
			if err != nil {
				return
			}

			if fieldType == "custom" {
				fieldTypeForm := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter the data type (You'll need to import it if necessary)").
							Value(&fieldType),
					),
				)
				err = fieldTypeForm.Run()
				if err != nil {
					return
				}
			}

			selectedAttrsForm := huh.NewForm(
				huh.NewGroup(
					huh.NewMultiSelect[string]().
						Title("Press x to select the attributes").
						Options(huh.NewOptions(required, unique)...).
						Value(&selectedAttrs),
				),
			)
			err = selectedAttrsForm.Run()
			if err != nil {
				return
			}

			fields = append(
				fields,
				&ModelField{
					Name:     fieldName,
					Type:     fieldType,
					Required: slices.Contains(selectedAttrs, required),
					Unique:   slices.Contains(selectedAttrs, unique),
				},
			)
		}

		mg := NewModelGenerator(&ModelConfig{Name: modelName, Fields: fields})
		err = mg.Generate()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("Model generated successfully.")
	},
}
