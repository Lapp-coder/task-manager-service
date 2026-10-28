package http

import (
	"net/http"

	"github.com/lapp-coder/task-manager-service/api"
)

const swaggerPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Task Manager API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => SwaggerUIBundle({ url: "/swagger/openapi.yaml", dom_id: "#swagger-ui" });
  </script>
</body>
</html>`

func swaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerPage))
}

func openAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(api.OpenAPI)
}
