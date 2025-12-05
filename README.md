# SMTP to Microsoft Graph Bridge

A high-performance, enterprise-ready SMTP server that receives emails via standard SMTP and relays them securely through the Microsoft Graph API.

Designed for legacy applications, printers, and devices that only support SMTP but need to send emails via Microsoft 365 using modern, secure **Certificate-Based Authentication**.

## Enterprise Features

-   **Modern Security:** Uses Azure AD Application Authentication (Client Credentials Flow) with PFX Certificates. Zero passwords stored in plain text.
-   **Flexible Config:** Supports `config.yaml`, Environment Variables, and `.env` files with strict hierarchy (Viper).
-   **Observability:**
    -   Structured JSON logging (ready for Splunk, ELK, Datadog).
    -   Health Check endpoint (`/health`) for Kubernetes/Load Balancers.
-   **Robust Parsing:** Full MIME support (HTML, Text, Encodings) powered by `go-message`.
-   **Docker Ready:** Stateless design, perfect for containers.

## Prerequisites

1.  **Microsoft 365 Tenant**
2.  **Azure AD App Registration:**
    -   Permission: `Mail.Send` (Application type).
    -   Admin Consent granted.
    -   Uploaded Certificate (Public Key).
3.  **PFX Certificate:** The matching private key file (with password) available to the bridge.

## Configuration

The application loads configuration in the following priority order (highest to lowest):
1.  **Environment Variables** (e.g., `MS_GRAPH_TENANT_ID`)
2.  **Config File** (`config.yaml` in current dir or `/etc/smtp-graph-bridge/`)
3.  **.env File** (Legacy/Dev support)
4.  **Default Values**

### Example `config.yaml`

```yaml
# Azure AD Configuration
ms_graph_tenant_id: "your-tenant-id"
ms_graph_client_id: "your-client-id"
ms_graph_cert_path: "./certs/cert.pfx"
ms_graph_cert_pass: "pfx-password"
ms_graph_email_from: "noreply@yourdomain.com"

# SMTP Server
smtp_port: 8025
smtp_host: "0.0.0.0" # Listen on all interfaces

# Observability
log_level: "info"    # debug, info, warn, error
health_port: 8080
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `MS_GRAPH_TENANT_ID` | Azure Directory ID |
| `MS_GRAPH_CLIENT_ID` | Azure Application ID |
| `MS_GRAPH_CERT_PATH` | Path to .pfx file |
| `MS_GRAPH_CERT_PASS` | PFX Password |
| `MS_GRAPH_EMAIL_FROM`| Sender address |
| `SMTP_PORT` | Port to listen on (default: 8025) |
| `LOG_LEVEL` | Log verbosity (default: info) |

## Installation & Run

### From Source

```bash
# Install dependencies
make install-deps

# Build
make build

# Run
./dist/smtp-graph-bridge
```

### Docker

```bash
docker build -t smtp-bridge .
docker run -d \
  -p 8025:8025 \
  -p 8080:8080 \
  -v $(pwd)/certs:/app/certs \
  -e MS_GRAPH_TENANT_ID=... \
  smtp-bridge
```

## Monitoring & Health

-   **Health Check:** `GET http://localhost:8080/health` (Returns 200 OK)
-   **Logs:** Outputs structured JSON to stdout.
    ```json
    {"time":"2023-10-27T10:00:00Z", "level":"INFO", "msg":"Email sent successfully", "recipient_count":1}
    ```

## Limitations

-   **Attachments:** Currently detected but **skipped**. Attachment support is planned for a future version.
-   **Auth:** SMTP Authentication (`AUTH PLAIN`) is supported but disabled by default.

## License

MIT License. See [LICENSE](LICENSE) file.