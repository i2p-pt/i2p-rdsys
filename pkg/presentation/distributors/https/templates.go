package https

import "text/template"

var indexPage = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html lang="{{.Lang}}">
<body>
<h1>{{.PageTitle}}</h1>
{{.VoilaBridges}}

{{range .Bridges}} {{.}} {{end}}

</body>
</html>
`))
