package entity

import "html/template"

const (
	cPersonTitle    = `{{.Name}}{{if (or .BirthDate .DeathDate)}} ({{if .BirthDate}}{{.BirthDate}}{{end}}-{{if .DeathDate}}{{.DeathDate}}{{end}}){{end}}`
	cPersonAbstract = ``
	cWorkTitle      = `{{if .ContribsBy "role/author"}}{{range .ContribsBy "role/author"}}{{.Agent.Name}}{{end}}: {{end}}{{.Title}}{{if .FirstPublicationDate}} ({{.FirstPublicationDate}}){{end}}`
	cWorkAbstract   = ``
)

var (
	tmplPersonTitle    = template.Must(template.New("person.title").Parse(cPersonTitle))
	tmplPersonAbstract = template.Must(template.New("person.abstract").Parse(cPersonAbstract))
	tmplWorkTitle      = template.Must(template.New("person.title").Parse(cWorkTitle))
	tmplWorkAbstract   = template.Must(template.New("person.abstract").Parse(cWorkAbstract))
)
