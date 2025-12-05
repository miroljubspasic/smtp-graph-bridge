.PHONY: build build-all clean run test install-deps

# Application name
APP_NAME=smtp-graph-bridge

# Build directory
BUILD_DIR=dist

# Version (can be overridden: make build VERSION=1.0.0)
VERSION?=0.1.0

# Go build flags
# Note: Removed -s flag to keep UUID on macOS (causes dyld abort without it)
LDFLAGS=-ldflags "-w -X main.version=${VERSION}"

# Default target
all: build

# Install dependencies
install-deps:
	@echo "Installing Go dependencies..."
	go mod download
	go mod tidy

# Build for current platform
build: clean
	@echo "Building ${APP_NAME} for current platform..."
	@mkdir -p ${BUILD_DIR}
	go build ${LDFLAGS} -o ${BUILD_DIR}/${APP_NAME} .
	@echo "Build complete: ${BUILD_DIR}/${APP_NAME}"

# Build for all platforms
build-all: clean install-deps
	@echo "Building ${APP_NAME} v${VERSION} for all platforms..."
	@mkdir -p ${BUILD_DIR}

	# Linux AMD64
	@echo "Building for Linux AMD64..."
	GOOS=linux GOARCH=amd64 go build ${LDFLAGS} -o ${BUILD_DIR}/${APP_NAME}-linux-amd64 .

	# Linux ARM64
	@echo "Building for Linux ARM64..."
	GOOS=linux GOARCH=arm64 go build ${LDFLAGS} -o ${BUILD_DIR}/${APP_NAME}-linux-arm64 .

	# macOS AMD64 (Intel)
	@echo "Building for macOS AMD64..."
	GOOS=darwin GOARCH=amd64 go build ${LDFLAGS} -o ${BUILD_DIR}/${APP_NAME}-darwin-amd64 .

	# macOS ARM64 (Apple Silicon)
	@echo "Building for macOS ARM64..."
	GOOS=darwin GOARCH=arm64 go build ${LDFLAGS} -o ${BUILD_DIR}/${APP_NAME}-darwin-arm64 .

	# Windows AMD64
	@echo "Building for Windows AMD64..."
	GOOS=windows GOARCH=amd64 go build ${LDFLAGS} -o ${BUILD_DIR}/${APP_NAME}-windows-amd64.exe .

	@echo "Build complete! Binaries are in ${BUILD_DIR}/"
	@ls -lh ${BUILD_DIR}/

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf ${BUILD_DIR}
	@echo "Clean complete"

# Run the application
run:
	@echo "Running ${APP_NAME}..."
	go run .

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	golangci-lint run

# Create release package
release: build-all
	@echo "Creating release packages..."
	@mkdir -p ${BUILD_DIR}/releases

	# Linux AMD64
	tar czf ${BUILD_DIR}/releases/${APP_NAME}-v${VERSION}-linux-amd64.tar.gz \
		-C ${BUILD_DIR} ${APP_NAME}-linux-amd64 \
		-C .. .env.example README.md

	# Linux ARM64
	tar czf ${BUILD_DIR}/releases/${APP_NAME}-v${VERSION}-linux-arm64.tar.gz \
		-C ${BUILD_DIR} ${APP_NAME}-linux-arm64 \
		-C .. .env.example README.md

	# macOS AMD64
	tar czf ${BUILD_DIR}/releases/${APP_NAME}-v${VERSION}-darwin-amd64.tar.gz \
		-C ${BUILD_DIR} ${APP_NAME}-darwin-amd64 \
		-C .. .env.example README.md

	# macOS ARM64
	tar czf ${BUILD_DIR}/releases/${APP_NAME}-v${VERSION}-darwin-arm64.tar.gz \
		-C ${BUILD_DIR} ${APP_NAME}-darwin-arm64 \
		-C .. .env.example README.md

	# Windows
	zip -j ${BUILD_DIR}/releases/${APP_NAME}-v${VERSION}-windows-amd64.zip \
		${BUILD_DIR}/${APP_NAME}-windows-amd64.exe \
		.env.example README.md

	@echo "Release packages created in ${BUILD_DIR}/releases/"
	@ls -lh ${BUILD_DIR}/releases/

# Display help
help:
	@echo "Available targets:"
	@echo "  make build         - Build for current platform"
	@echo "  make build-all     - Build for all platforms"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make run           - Run the application"
	@echo "  make test          - Run tests"
	@echo "  make fmt           - Format code"
	@echo "  make lint          - Lint code"
	@echo "  make release       - Create release packages"
	@echo "  make install-deps  - Install Go dependencies"
	@echo "  make help          - Display this help message"
