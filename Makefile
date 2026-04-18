COMPOSE := docker compose -f docker-compose.yml -f docker-compose.e2e.yml

.PHONY: e2e e2e-up e2e-deps e2e-test e2e-down

e2e:
	@set -e; \
	if [ "$(KEEP)" != "1" ]; then trap '$(COMPOSE) down' EXIT; fi; \
	$(COMPOSE) up --build -d; \
	until curl --silent --fail http://127.0.0.1:8032/ >/dev/null; do \
		sleep 1; \
	done; \
	npm install; \
	npx playwright test

e2e-up:
	$(COMPOSE) up --build -d

e2e-deps:
	npm install

e2e-test:
	npx playwright test

e2e-down:
	$(COMPOSE) down
