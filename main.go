// main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Field represents a field in a Django model.
type Field struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Nullable  bool   `json:"nullable"`
	Unique    bool   `json:"unique"`
	Relation  string `json:"relation,omitempty"`
	RelatedTo string `json:"related_to,omitempty"`
}

// Model represents a Django model with its fields.
type Model struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

// Output represents the output from the Python parser, including models and queries.
type Output struct {
	Models  []Model  `json:"models"`
	Queries []string `json:"queries"`
}

// main is the entry point of the CLI application.
func main() {
	input := flag.String("input", "", "Path to Django app (required)")
	output := flag.String("output", "./out", "Output directory")
	dialect := flag.String("dialect", "postgres", "SQL dialect: postgres or mysql")
	dryRun := flag.Bool("dry-run", false, "Dry run mode (prints output to stdout without writing files)")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage of %s:
`, os.Args[0])
		fmt.Println("A CLI tool to convert Django models into SQL and sqlc configurations.")
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println(`
Example:
  go run main.go --input ./myapp --output ./out --dialect postgres
`)
	}

	flag.Parse()

	if *input == "" {
		fmt.Println("Error: --input is required")
		flag.Usage()
		os.Exit(1)
	}

	// Run Python parser
	out, err := runPythonParser(*input)
	if err != nil {
		fmt.Printf("Parser error: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Println("=== Models ===")
		for _, m := range out.Models {
			fmt.Printf("%s: %+v\n", m.Name, m.Fields)
		}
		fmt.Println("=== Queries ===")
		for _, q := range out.Queries {
			fmt.Println(q)
		}
		return
	}

	// Prepare output directories
	migrations := filepath.Join(*output, "migrations")
	os.MkdirAll(migrations, 0755)

	// Generate and write files
	write(filepath.Join(*output, "schema.sql"), generateSQL(out.Models, *dialect))
	write(filepath.Join(migrations, timestamp()+"_create_tables.up.sql"), generateSQL(out.Models, *dialect))
	write(filepath.Join(migrations, timestamp()+"_create_tables.down.sql"), generateDownSQL(out.Models, *dialect))
	write(filepath.Join(*output, "query.sql"), strings.Join(out.Queries, "\n\n"))
	write(filepath.Join(*output, "sqlc.yaml"), generateSQLCConfig(*dialect))

	fmt.Println("✅ Generated schema.sql, migrations, query.sql, sqlc.yaml")
}

// runPythonParser executes the embedded Python script on the specified Django app path.
func runPythonParser(path string) (*Output, error) {
	cmd := exec.Command("python3", "-c", pythonScript(), path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var result Output
	err := json.Unmarshal(out.Bytes(), &result)
	return &result, err
}

// write writes content to a file at the given path.
func write(path string, content string) {
	os.WriteFile(path, []byte(content), 0644)
}

// timestamp returns a formatted timestamp string for file naming.
func timestamp() string {
	return time.Now().Format("20060102150405")
}

// generateSQL generates CREATE TABLE SQL for the given models.
func generateSQL(models []Model, dialect string) string {
	var sb strings.Builder
	for _, m := range models {
		sb.WriteString("CREATE TABLE " + toSnake(m.Name) + " (\n")
		sb.WriteString("    id SERIAL PRIMARY KEY,\n")
		for _, f := range m.Fields {
			col := "    " + toSnake(f.Name) + " " + sqlType(f.Type, dialect)
			if !f.Nullable {
				col += " NOT NULL"
			}
			if f.Unique {
				col += " UNIQUE"
			}
			sb.WriteString(col + ",\n")
		}
		for _, f := range m.Fields {
			if f.Relation == "foreignkey" || f.Relation == "one2one" {
				sb.WriteString(fmt.Sprintf("    FOREIGN KEY (%s_id) REFERENCES %s(id),\n",
					toSnake(f.Name), toSnake(f.RelatedTo)))
			}
		}
		sb.Truncate(sb.Len() - 2)
		sb.WriteString("\n);\n\n")

		for _, f := range m.Fields {
			if f.Relation == "many2many" {
				join := toSnake(m.Name) + "_" + toSnake(f.Name)
				sb.WriteString(fmt.Sprintf(
					"CREATE TABLE %s (\n    %s_id INTEGER REFERENCES %s(id),\n    %s_id INTEGER REFERENCES %s(id)\n);\n\n",
					join, toSnake(m.Name), toSnake(m.Name), toSnake(f.RelatedTo), toSnake(f.RelatedTo),
				))
			}
		}
	}
	return sb.String()
}

// generateDownSQL generates DROP TABLE SQL statements for the models.
func generateDownSQL(models []Model, dialect string) string {
	var sb strings.Builder
	for _, m := range models {
		for _, f := range m.Fields {
			if f.Relation == "many2many" {
				sb.WriteString("DROP TABLE IF EXISTS " + toSnake(m.Name) + "_" + toSnake(f.Name) + ";\n")
			}
		}
		sb.WriteString("DROP TABLE IF EXISTS " + toSnake(m.Name) + ";\n")
	}
	return sb.String()
}

// sqlType maps Django field types to SQL types based on dialect.
func sqlType(ftype, dialect string) string {
	switch ftype {
	case "CharField", "TextField":
		return "TEXT"
	case "IntegerField":
		return "INTEGER"
	case "FloatField":
		return "REAL"
	case "BooleanField":
		return "BOOLEAN"
	case "DateField", "DateTimeField":
		return "TIMESTAMP"
	default:
		return "TEXT"
	}
}

// toSnake converts a string to snake_case.
func toSnake(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
}

// generateSQLCConfig returns a sqlc.yaml configuration string.
func generateSQLCConfig(dialect string) string {
	return fmt.Sprintf(`version: "2"
sql:
  - engine: %s
    queries: "./query.sql"
    schema: "./schema.sql"
    gen:
      go:
        package: "db"
        out: "./db"
`, dialect)
}

// pythonScript returns the embedded Python script as a string.
func pythonScript() string {
	return `
import sys, os, ast, json

def extract_models(path: str):
    result = []
    queries = []
    for root, _, files in os.walk(path):
        for file in files:
            if file.endswith(".py"):
                full = os.path.join(root, file)
                with open(full) as f:
                    tree = ast.parse(f.read(), filename=full)
                for node in tree.body:
                    if isinstance(node, ast.ClassDef):
                        bases = [b.id if isinstance(b, ast.Name) else "" for b in node.bases]
                        if "Model" in bases:
                            fields = []
                            for stmt in node.body:
                                if isinstance(stmt, ast.Assign) and isinstance(stmt.value, ast.Call):
                                    fname = stmt.targets[0].id
                                    ftype = stmt.value.func.attr if isinstance(stmt.value.func, ast.Attribute) else ""
                                    kwargs = {k.arg: getattr(k.value, 's', getattr(k.value, 'value', None)) for k in stmt.value.keywords}
                                    nullable = kwargs.get('null', False)
                                    unique = kwargs.get('unique', False)
                                    related = None
                                    to = None
                                    if ftype in ["ForeignKey", "OneToOneField", "ManyToManyField"]:
                                        related = ftype.lower().replace("field", "")
                                        to = stmt.value.args[0].id if isinstance(stmt.value.args[0], ast.Name) else ""
                                    fields.append({
                                        "name": fname,
                                        "type": ftype,
                                        "nullable": nullable,
                                        "unique": unique,
                                        "relation": related,
                                        "related_to": to
                                    })
                            result.append({"name": node.name, "fields": fields})
                with open(full) as f:
                    code = f.read()
                    if ".objects." in code:
                        for line in code.splitlines():
                            if ".objects." in line and ("filter(" in line or "get(" in line or "create(" in line):
                                queries.append("-- from: %s\n-- %s" % (file, line.strip()))
    print(json.dumps({"models": result, "queries": queries}))

extract_models(sys.argv[1])
`
}
