# gogogo

一个使用 Go 标准库 + SQLite 搭建的轻量 CRUD API 示例项目。

## 功能

- `GET /health`：健康检查
- `GET /items`：查询列表
- `POST /items`：创建数据
- `GET /items/{id}`：查询详情
- `PUT /items/{id}`：更新数据
- `DELETE /items/{id}`：删除数据

## 运行

```bash
API_TOKEN=dev-secret go run ./cmd/api
```

默认监听 `:8080`，数据会写入当前目录的 `data.db`。也可以通过环境变量修改：

```bash
API_TOKEN=dev-secret HTTP_ADDR=:3000 DATABASE_DSN=./storage/app.db go run ./cmd/api
```

`GET /items` 可以公开访问；`POST /items`、`PUT /items/{id}`、`DELETE /items/{id}` 必须携带：

```http
Authorization: Bearer dev-secret
```

服务启动时会自动创建数据表：

```sql
CREATE TABLE IF NOT EXISTS items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
```

## 示例请求

```bash
curl -X POST http://localhost:8080/items \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev-secret' \
  -d '{"name":"demo","description":"first item"}'

curl http://localhost:8080/items

curl -X PUT http://localhost:8080/items/1 \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev-secret' \
  -d '{"name":"updated","description":"updated item"}'

curl -X DELETE http://localhost:8080/items/1 \
  -H 'Authorization: Bearer dev-secret'
```

## 测试

```bash
go test ./...
```
