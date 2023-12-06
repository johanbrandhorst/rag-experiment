database-up:
	docker run -p 5432:5432 --rm --name postgres -e POSTGRES_PASSWORD=postgres -d ankane/pgvector

database-down:
	docker stop postgres

psql:
	docker run -it --rm --link postgres:postgres postgres psql postgresql://postgres:postgres@postgres:5432/postgres

gen:
	docker run --rm -v $(shell pwd):/src/ -w /src/postgres sqlc/sqlc generate
	sudo chown $(shell whoami): -R ./postgres
