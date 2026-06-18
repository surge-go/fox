package main

import (
	"log"
	"net/http"
	"time"

	"github.com/surge-go/fox"
	"github.com/surge-go/fox/middleware"
)

type createUserRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type AppController struct{}

func (ctl *AppController) Ping(c *fox.Context) {
	c.String(http.StatusOK, "pong")
}

func (ctl *AppController) Hello(c *fox.Context) {
	name := c.DefaultQuery("name", "fox")
	c.Ok(map[string]any{
		"message": "hello " + name,
	})
}

func (ctl *AppController) CreateUser(c *fox.Context) {
	var req createUserRequest
	if err := c.BindJSON(&req); err != nil {
		return
	}

	c.Ok(map[string]any{
		"name": req.Name,
		"age":  req.Age,
	})
}

func main() {
	printRoutes := true
	app := fox.New(&fox.Config{
		Mode:            fox.ModeDebug,
		Addr:            ":8080",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		PrintRoutes:     &printRoutes,
	})

	// 全局中间件会作用于后续注册的所有路由。
	app.Use(middleware.Logger())
	app.Use(func(c *fox.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = time.Now().Format("20060102150405.000000000")
		}
		c.SetTraceID(traceID)
		c.SetHeader("X-Trace-ID", traceID)
		c.Next()
	})

	controller := &AppController{}
	app.GET("/ping", controller.Ping)

	api := app.Group("/api")
	api.GET("/hello", controller.Hello)
	api.POST("/users", controller.CreateUser)
	api.GET("/t", T)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func T(ctx *fox.Context) {
	ctx.JSON(200, map[string]interface{}{
		"hello": "world",
	})
}
