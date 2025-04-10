# Django-to-sqlc CLI

A Go CLI that converts Django models into:

- SQL schema definitions
- SQL migrations using [go-migrate](https://github.com/golang-migrate/migrate)
- `query.sql` from Python `.objects.filter()`/`.get()`/`.create()` calls
- `sqlc.yaml` config for generating Go DB code via [sqlc](https://sqlc.dev)

## Features

- ✅ Supports Django field types and relationships:
  - `ForeignKey`, `OneToOneField`, `ManyToManyField`
- ✅ Parses all `.py` files in the Django app to extract models and common ORM queries
- ✅ Generates:
  - `schema.sql`
  - Timestamped `up.sql` and `down.sql` migration files
  - `query.sql`
  - `sqlc.yaml`
- ✅ CLI flags:
  - `--input` Django app path (required)
  - `--output` output directory (default: `./out`)
  - `--dialect` SQL dialect: `postgres` (default) or `mysql`
  - `--dry-run` shows what would be generated without writing files

## Installation

```
go build -o django-sqlc main.go
```

## Usage

```bash
./django-sqlc --input ./my_django_app --output ./generated --dialect postgres
```

With dry-run mode:

```bash
./django-sqlc --input ./my_django_app --dry-run
```

## Output

When run, the tool creates:

```text
./out/
├── migrations/
│   ├── 20250410131500_create_tables.up.sql
│   └── 20250410131500_create_tables.down.sql
├── query.sql
├── schema.sql
└── sqlc.yaml
```

## Example Django model

```python
class Book(models.Model):
    title = models.CharField(max_length=255)
    author = models.ForeignKey("Author", on_delete=models.CASCADE)
    tags = models.ManyToManyField("Tag")
```

### Output SQL (PostgreSQL)

```sql
CREATE TABLE book (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    author_id INTEGER NOT NULL,
    FOREIGN KEY (author_id) REFERENCES author(id)
);

CREATE TABLE book_tags (
    book_id INTEGER REFERENCES book(id),
    tag_id INTEGER REFERENCES tag(id)
);
```

## Notes

- Only standard Django ORM is supported.
- Queries are basic; customize `query.sql` for more complex behavior.
- Relationships require both ends of the relation to be declared explicitly.

## License

MIT
