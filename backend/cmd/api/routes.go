package main

import "net/http"

func (app *Application) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", app.healthHandler)

	mux.HandleFunc("POST /auth/register", app.registerHandler)
	mux.HandleFunc("POST /auth/login", app.loginHandler)
	mux.HandleFunc("GET /users", app.getUsersHandler)

	mux.HandleFunc(
		"GET /wallet",
		app.requireAuth(app.getWalletHandler),
	)

	mux.HandleFunc(
		"POST /topups",
		app.requireAuth(app.createTopUpHandler),
	)

	mux.HandleFunc(
		"POST /transfers",
		app.requireAuth(app.createTransferHandler),
	)

	mux.HandleFunc(
		"POST /transfers/{id}/refunds",
		app.requireAuth(app.handleRefundTransfer),
	)

	// Register webhook endpoint in your Go main router setup
	mux.HandleFunc("POST /webhooks/midtrans", app.midtransWebhookHandler)

	return app.enableCORS(mux)
}

//NewServeMux returns a new ServeMux with the registered routes.
//HandleFunc is used to register the routes with the ServeMux. The routes are defined in the routes() method of the Application struct.
//The routes include health check, user creation, wallet retrieval, top-up creation, and transfer creation.
