# Contributing

This project is currently in active development and **not accepting external contributions**.

Feel free to:
- Open issues for bug reports
- Suggest features via issues
- Fork for personal use

Once the project reaches a stable release, contribution guidelines will be added.

## Development

Run `make help` to see all available commands:

```bash
make help
```

Key targets:

| Target       | Description                              |
|--------------|------------------------------------------|
| `make build` | Build the binary                         |
| `make test`  | Run unit tests                           |
| `make check` | Run all checks (fmt, vet, lint, test)    |
| `make tools` | Install staticcheck and gosec            |

See [docs/LAYOUT.md](docs/LAYOUT.md) for project structure and test conventions.
