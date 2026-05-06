# Dockerfile for agentX sandbox
FROM python:3.12-slim

# Install system dependencies if needed (none for now)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy metadata and install dependencies
COPY pyproject.toml .
RUN pip install --no-cache-dir .[dev]

# Copy source code
COPY . .

# Expose application port
EXPOSE 8000

# Default command to run the FastAPI app
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
