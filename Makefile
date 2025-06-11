.PHONY: build run docker-build docker-run clean setup-docker

# Local development
setup-local:
	chmod +x scripts/setup-local.sh
	./scripts/setup-local.sh

run-local: setup-local
	go run cmd/api/main.go

# Docker development
setup-docker:
	chmod +x scripts/setup-docker.sh
	./scripts/setup-docker.sh

docker-run:
	docker compose up --build

docker-stop:
	docker compose down

docker-clean:
	docker compose down -v
	docker system prune -f

# Build
build:
	go build -o bin/main cmd/api/main.go

# Clean
clean:
	rm -rf bin/
	docker compose down -v