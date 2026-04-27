package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fasthttp/router"
	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	log.SetPrefix("main: ")
	if os.Getenv("DEBUG") == "true" {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile | log.Lmsgprefix)
	} else {
		log.SetFlags(0)
	}

	r := router.New()
	r.GET("/ready", isReady)

	println("application started")

	if os.Getenv("DEBUG") == "true" {
		log.Fatal(fasthttp.ListenAndServe(":1234", Logger(r.Handler)))
	} else {
		log.Fatal(fasthttp.ListenAndServe(":1234", r.Handler))
	}
}

func Logger(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	// 2026-02-22 21:27:26 | GET "/ready" | 200 | 3.24189ms
	return func(ctx *fasthttp.RequestCtx) {
		startTime := time.Now()
		next(ctx)
		duration := time.Since(startTime)
		fmt.Printf(
			"%s | %s \"%s\" | %d | %s\n",
			startTime.UTC().Add(-3*time.Hour).Format(time.DateTime),
			ctx.Method(),
			ctx.Path(),
			ctx.Response.StatusCode(),
			duration,
		)
	}
}

func isReady(ctx *fasthttp.RequestCtx) {
	ready := true
	if ready {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		return
	} else {
		ctx.Response.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return
	}
}
