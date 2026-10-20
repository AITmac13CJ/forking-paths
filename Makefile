.PHONY: db api web publish

db:
	docker compose up -d

api:
	cd backend && go run ./cmd/server

web:
	cd frontend && npm run dev

publish:
	./deploy/publish.sh
