"""
Service manager for AgentX external services (Ollama, Agentix).

Handles startup, health checks, and graceful shutdown of required services.
"""

import logging
import subprocess
import time
from dataclasses import dataclass
from typing import Optional

import httpx

logger = logging.getLogger(__name__)


@dataclass
class ServiceConfig:
    """Configuration for an external service."""

    name: str
    """Service name (e.g., 'ollama', 'agentix')."""

    host: str
    """Host address (e.g., 'localhost')."""

    port: int
    """Port number (e.g., 11435)."""

    health_endpoint: str
    """Relative endpoint to check service health (e.g., '/api/models')."""

    start_command: Optional[list] = None
    """Command to start service as subprocess (e.g., ['ollama', 'serve'])."""

    @property
    def url(self) -> str:
        """Full URL to service."""
        return f"http://{self.host}:{self.port}"


class ServiceManager:
    """
    Manages external services required by AgentX.

    Handles:
    - Health checks via HTTP endpoints
    - Service startup if not running
    - Graceful shutdown
    - Connection configuration
    """

    def __init__(self, config: dict):
        """
        Initialize service manager with AgentX configuration.

        Args:
            config: AgentX configuration dictionary with agentx section
        """
        self.config = config
        self.services: dict[str, ServiceConfig] = {}
        self._started_processes: list[subprocess.Popen] = []
        self._setup_services()

    def _setup_services(self) -> None:
        """Configure known services from AgentX config."""
        agentx_config = self.config.get("agentx", {})

        # Ollama service
        ollama_host = agentx_config.get("ollama_host", "localhost:11435")
        host, port = self._parse_host_port(ollama_host, 11435)

        self.services["ollama"] = ServiceConfig(
            name="Ollama", host=host, port=port, health_endpoint="/api/models", start_command=["ollama", "serve"]
        )

        # Agentix service (always integrated)
        agentix_host = self.config.get("agentix", {}).get("host", "localhost:8000")
        host, port = self._parse_host_port(agentix_host, 8000)

        self.services["agentix"] = ServiceConfig(
            name="Agentix",
            host=host,
            port=port,
            health_endpoint="/health",
            start_command=["python", "-m", "agentix.server", "--port", str(port)],
        )

    @staticmethod
    def _parse_host_port(host_string: str, default_port: int) -> tuple[str, int]:
        """
        Parse host:port string into components.

        Args:
            host_string: "host:port" or "host"
            default_port: Port to use if not specified

        Returns:
            Tuple of (host, port)
        """
        if ":" in host_string:
            host, port_str = host_string.rsplit(":", 1)
            try:
                return host, int(port_str)
            except ValueError:
                return host_string, default_port
        return host_string, default_port

    def check_health(self, service_name: str) -> bool:
        """
        Check if a service is healthy via HTTP health endpoint.

        Args:
            service_name: Name of service to check ('ollama', 'agentix')

        Returns:
            True if service responds, False otherwise
        """
        if service_name not in self.services:
            return False

        service = self.services[service_name]
        url = f"{service.url}{service.health_endpoint}"

        try:
            with httpx.Client(timeout=2.0) as client:
                response = client.get(url)
                return response.status_code < 500
        except (httpx.RequestError, Exception):
            return False

    def start_service(self, service_name: str, timeout: int = 30) -> bool:
        """
        Start a service if not already running.

        Args:
            service_name: Name of service to start ('ollama', 'agentix')
            timeout: Seconds to wait for service to become healthy

        Returns:
            True if service is running, False if startup failed
        """
        if service_name not in self.services:
            return False

        service = self.services[service_name]

        # Check if already healthy
        if self.check_health(service_name):
            logger.info("%s is already running", service.name)
            return True

        # Try to start if command available
        if not service.start_command:
            logger.warning("Cannot start %s - no start command configured", service.name)
            return False

        logger.info("Starting %s...", service.name)
        try:
            process = subprocess.Popen(service.start_command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self._started_processes.append(process)
        except FileNotFoundError:
            logger.error("%s command not found: %s", service.name, service.start_command[0])
            return False
        except Exception as e:
            logger.exception("Failed to start %s: %s", service.name, e)
            return False

        # Wait for service to become healthy
        start_time = time.time()
        while time.time() - start_time < timeout:
            if self.check_health(service_name):
                logger.info("%s started successfully", service.name)
                return True

            # Check if process has already crashed
            poll_result = process.poll()
            if poll_result is not None:
                # Process exited prematurely
                try:
                    _, stderr = process.communicate(timeout=1)
                    if stderr:
                        error_msg = stderr.decode("utf-8", errors="replace").strip()
                        if error_msg:
                            logger.error("  Error details: %s", error_msg[:200])
                except Exception as exc:
                    logger.debug("Could not read stderr from crashed %s process: %s", service.name, exc)
                logger.error("%s process exited prematurely (exit code: %s)", service.name, poll_result)
                return False

            time.sleep(0.5)

        # Check if process is still running but unhealthy
        if process.poll() is None:
            logger.error("%s did not start within %s seconds (process running but unhealthy)", service.name, timeout)
        else:
            try:
                _, stderr = process.communicate(timeout=1)
                if stderr:
                    error_msg = stderr.decode("utf-8", errors="replace").strip()
                    if error_msg:
                        logger.error("  Error output: %s", error_msg[:200])
            except Exception as exc:
                logger.debug("Could not read stderr from timed-out %s process: %s", service.name, exc)
            logger.error("%s did not start (process crashed)", service.name)

        return False

    def ensure_services(self, services: list[str], timeout: int = 30) -> bool:
        """
        Ensure all required services are running.

        Args:
            services: List of service names to start ('ollama', 'agentix')
            timeout: Seconds to wait for each service

        Returns:
            True if all services started successfully
        """
        all_started = True
        for service_name in services:
            if not self.start_service(service_name, timeout):
                all_started = False
        return all_started

    def shutdown(self) -> None:
        """Gracefully shutdown all started services."""
        for process in self._started_processes:
            if process.poll() is None:  # Process still running
                try:
                    process.terminate()
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                except Exception as e:
                    logger.error("Error stopping process: %s", e)

    def get_service_url(self, service_name: str) -> Optional[str]:
        """
        Get full URL for a service.

        Args:
            service_name: Name of service ('ollama', 'agentix')

        Returns:
            Full URL (e.g., 'http://localhost:11435') or None if not found
        """
        if service_name not in self.services:
            return None
        return self.services[service_name].url
