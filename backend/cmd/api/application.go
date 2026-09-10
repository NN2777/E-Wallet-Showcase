package main

import "github.com/jackc/pgx/v5/pgxpool"

type Application struct {
	db *pgxpool.Pool
}