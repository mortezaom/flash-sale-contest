#!/bin/bash

echo "Setting up Docker environment..."

# Build and start services
COMPOSE_BAKE=true docker compose up --build -d

# Wait for services to be healthy
echo "Waiting for services to be ready..."
docker compose exec psql_bp pg_isready -U postgres
docker compose exec keydb redis-cli ping

echo "Docker setup complete!"
echo "Services are running at:"
echo "  - API: http://localhost:8080"
echo "  - PostgreSQL: localhost:5432"
echo "  - Redis: localhost:6379"