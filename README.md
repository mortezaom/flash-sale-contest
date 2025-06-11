# High-Throughput Flash Sale Service - NOT Back Contest

![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg) ![Build Status](https://img.shields.io/badge/build-passing-brightgreen) ![License](https://img.shields.io/badge/license-MIT-green)

A high-performance flash sale service built from scratch to sell exactly 10,000 items per hour. Built with Go, Redis, and PostgreSQL, using zero frameworks.

## ✨ Core Features

-   **No Overselling**: Atomic Redis operations guarantee inventory correctness under extreme load.
-   **No Underselling**: Automatically recovers items from failed purchases, ensuring all 10,000 items are sold.
-   **High Throughput**: Load tested with an average response time between 1-80ms.
-   **Automated Sale Rotation**: A new sale with 10,000 items starts automatically every hour.
-   **Rock-Solid Stability**: Includes graceful shutdown, recovery, and rate limiting to prevent crashes.
-   **Full Observability**: A `/metrics` endpoint provides real-time system health.

## 🚀 Getting Started (Docker)

The entire stack runs with a single command.

**Prerequisites**:
-   Docker
-   Docker Compose

### Instructions

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/mortezaom/flash-sale-contest.git
    cd flash-sale-contest
    ```

2.  **Copy/Change the environment file:**
    ```bash
    cp .env.example .env
    ```
    or change the values if .env existed

3.  **Build and run the services:**
    ```bash
    make setup-docker
    ```
    This command builds the containers, starts the services, and runs database migrations automatically.

## 🧪 Testing & Verification

After setup, the system is live. Here’s how to test it:

1.  **Check Initial Status:**
    ```bash
    curl http://localhost:8080/sale/status
    ```
    > **Expect:** A JSON response with `"remaining_items": 10000`.

2.  **Run the Load Test:**
    This test simulates the full checkout-and-purchase flow under heavy load.
    ```bash
    # First, install k6 (e.g., brew install k6)
    k6 run load-test.js
    ```

3.  **Verify the Final Count:**
    After the test, check the database for the final count of purchased items. (replase user and db if needed)
    ```bash
    docker compose exec psql_bp psql -U root -d blueprint -c "SELECT COUNT(*) FROM purchases;"
    ```
    > **Expect:** The count will be **exactly `10000`**, proving the system's correctness.

## 🔌 API Endpoints

-   **Checkout an Item**
    ```bash
    curl -X POST "http://localhost:8080/checkout?user_id=user-123&id=item-abc"
    ```

-   **Purchase an Item**
    ```bash
    curl -X POST "http://localhost:8080/purchase?code=<checkout_code>"
    ```

-   **Get Sale Status**
    ```bash
    curl http://localhost:8080/sale/status
    ```

## 🛠️ Tech Stack

-   **Language**: Go (stdlib http, pgx, go-redis)
-   **Database**: PostgreSQL
-   **In-Memory Store**: Redis (KeyDB)
-   **Containerization**: Docker & Docker Compose
-   **Load Testing**: k6
-   **Template Used**: https://go-blueprint.dev/ 