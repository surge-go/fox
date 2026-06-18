# Fox Example

运行示例：

```bash
go run ./example
```

测试接口：

```bash
curl http://localhost:8080/ping
curl "http://localhost:8080/api/hello?name=surge"
curl -X POST http://localhost:8080/api/users \
  -H 'Content-Type: application/json' \
  -d '{"name":"aikzy","age":18}'
```
