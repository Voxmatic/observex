# Contributing to ObserveX

## Development setup

```bash
# Prerequisites: Go 1.22+, Node 20+, Docker, kubectl (optional)
git clone https://github.com/your-org/observex.git
cd observex

# Install tools
make install-tools

# Start dev stack
make dev

# Frontend hot reload (separate terminal)
make dev-frontend
```

## Code style

- **Go**: `gofmt` + `golangci-lint`. Run `make fmt && make lint` before committing.
- **TypeScript**: ESLint config in `frontend/.eslintrc`. Run `make lint-frontend`.
- **Commits**: Conventional Commits format — `feat:`, `fix:`, `docs:`, `chore:`.

## Adding a new backend feature

1. Add models to `pkg/models/models.go`
2. Add DB migration if needed: `internal/db/migrations/NNN_description.sql`
3. Add handler to the appropriate service
4. Wire route in `services/api-gateway/main.go`
5. Add API client in `frontend/src/lib/api.ts`
6. Write tests

## Adding a new frontend page

1. Create `frontend/src/pages/MyPage.tsx`
2. Import and add route in `frontend/src/App.tsx`
3. Add nav item in `frontend/src/components/shared/Layout.tsx`

## Testing

```bash
make test              # unit tests
make test-integration  # integration tests (needs Postgres)
make test-cover        # coverage report
```

## Pull request checklist

- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] New features have tests
- [ ] API changes are reflected in `api.ts`
- [ ] DB changes have a migration
- [ ] README updated if needed
