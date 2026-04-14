.PHONY: run run-backend run-frontend 

BACKEND_PORT  := 8080
FRONTEND_PORT := 5173


run: ## Run backend and frontend together
	@echo "Starting Raft Simulator"
	@$(MAKE) -j2 run-backend run-frontend

run-backend: ## Run the Go backend
	@echo "Backend → http://localhost:$(BACKEND_PORT)/control  (ws: /ws)"
	@cd backend && go run main.go

run-frontend: ## Run the Vite frontend
	@echo "Frontend → http://localhost:$(FRONTEND_PORT)"
	@cd frontend && npm run dev
