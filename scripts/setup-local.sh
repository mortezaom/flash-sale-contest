#!/bin/bash

echo "Setting up local development environment..."

# Check if PostgreSQL is running
if ! command -v psql &> /dev/null; then
    echo "PostgreSQL is not installed. Please install PostgreSQL first."
    exit 1
fi

# Check if Redis is running
if ! command -v redis-cli &> /dev/null; then
    echo "Redis is not installed. Please install Redis first."
    exit 1
fi

# Load environment variables
source .env

# Create database if it doesn't exist
echo "Creating database if it doesn't exist..."
createdb -h $NOTBACK_DB_HOST -p $NOTBACK_DB_PORT -U $NOTBACK_DB_USERNAME $NOTBACK_DB_DATABASE 2>/dev/null || echo "Database already exists"

# Run migrations
echo "Running migrations..."
go run cmd/migrate/main.go

# Start Redis if not running
redis-cli ping > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "Starting Redis..."
    redis-server --daemonize yes
fi

echo "Local setup complete!"
echo "You can now run: go run cmd/api/main.go"