"""
Tests for ServiceManager - external service (Ollama, Agentix) management.
"""

import subprocess
import unittest
from unittest.mock import Mock, patch, MagicMock
from agentx.service_manager import ServiceManager, ServiceConfig


class TestServiceConfig(unittest.TestCase):
    """Test ServiceConfig data class."""
    
    def test_service_config_creation(self):
        """Test ServiceConfig creation with required fields."""
        config = ServiceConfig(
            name="Ollama",
            host="localhost",
            port=11435,
            health_endpoint="/api/models"
        )
        
        self.assertEqual(config.name, "Ollama")
        self.assertEqual(config.host, "localhost")
        self.assertEqual(config.port, 11435)
        self.assertEqual(config.health_endpoint, "/api/models")
        self.assertIsNone(config.start_command)
    
    def test_service_config_url(self):
        """Test URL property generation."""
        config = ServiceConfig(
            name="Test",
            host="example.com",
            port=8080,
            health_endpoint="/health"
        )
        
        self.assertEqual(config.url, "http://example.com:8080")


class TestParseHostPort(unittest.TestCase):
    """Test host:port parsing utility."""
    
    def test_parse_host_port_with_port(self):
        """Test parsing 'host:port' format."""
        host, port = ServiceManager._parse_host_port("localhost:11435", 9999)
        self.assertEqual(host, "localhost")
        self.assertEqual(port, 11435)
    
    def test_parse_host_port_without_port(self):
        """Test parsing 'host' only uses default port."""
        host, port = ServiceManager._parse_host_port("localhost", 8000)
        self.assertEqual(host, "localhost")
        self.assertEqual(port, 8000)
    
    def test_parse_host_port_with_ipv6(self):
        """Test parsing IPv6-like addresses."""
        host, port = ServiceManager._parse_host_port("::1:8000", 9999)
        self.assertEqual(host, "::1")
        self.assertEqual(port, 8000)
    
    def test_parse_host_port_invalid_port(self):
        """Test parsing with invalid port falls back to default."""
        host, port = ServiceManager._parse_host_port("localhost:notaport", 8000)
        self.assertEqual(host, "localhost:notaport")
        self.assertEqual(port, 8000)


class TestServiceManager(unittest.TestCase):
    """Test ServiceManager initialization and configuration."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.config = {
            "agentx": {
                "ollama_host": "localhost:11435",
                "ollama_model": "gpt-oss",
            },
            "agentix": {
                "host": "localhost:8000",
            }
        }
    
    def test_initialization(self):
        """Test ServiceManager initialization."""
        manager = ServiceManager(self.config)
        
        self.assertIn("ollama", manager.services)
        self.assertIn("agentix", manager.services)
    
    def test_ollama_service_config(self):
        """Test Ollama service is configured correctly."""
        manager = ServiceManager(self.config)
        ollama = manager.services["ollama"]
        
        self.assertEqual(ollama.name, "Ollama")
        self.assertEqual(ollama.host, "localhost")
        self.assertEqual(ollama.port, 11435)
        self.assertEqual(ollama.health_endpoint, "/api/models")
        self.assertEqual(ollama.start_command, ["ollama", "serve"])
    
    def test_agentix_service_config(self):
        """Test Agentix service is configured."""
        manager = ServiceManager(self.config)
        agentix = manager.services["agentix"]
        
        self.assertEqual(agentix.name, "Agentix")
        self.assertEqual(agentix.host, "localhost")
        self.assertEqual(agentix.port, 8000)
        self.assertEqual(agentix.health_endpoint, "/health")
        self.assertEqual(agentix.start_command, ["python", "-m", "agentix.server", "--port", "8000"])
    
    def test_get_service_url(self):
        """Test getting full service URL."""
        manager = ServiceManager(self.config)
        
        ollama_url = manager.get_service_url("ollama")
        self.assertEqual(ollama_url, "http://localhost:11435")
        
        agentix_url = manager.get_service_url("agentix")
        self.assertEqual(agentix_url, "http://localhost:8000")
    
    def test_get_nonexistent_service_url(self):
        """Test getting URL for non-existent service returns None."""
        manager = ServiceManager(self.config)
        
        url = manager.get_service_url("nonexistent")
        self.assertIsNone(url)


class TestHealthCheck(unittest.TestCase):
    """Test service health checking."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.config = {
            "agentx": {"ollama_host": "localhost:11435"},
            "agentix": {"host": "localhost:8000"}
        }
        self.manager = ServiceManager(self.config)
    
    @patch('httpx.Client')
    def test_health_check_success(self, mock_client_class):
        """Test successful health check."""
        mock_response = Mock()
        mock_response.status_code = 200
        
        mock_client = MagicMock()
        mock_client.__enter__.return_value.get.return_value = mock_response
        mock_client_class.return_value = mock_client
        
        result = self.manager.check_health("ollama")
        
        self.assertTrue(result)
        mock_client.__enter__.return_value.get.assert_called_once()
    
    @patch('httpx.Client')
    def test_health_check_server_error(self, mock_client_class):
        """Test health check with server error."""
        mock_response = Mock()
        mock_response.status_code = 503
        
        mock_client = MagicMock()
        mock_client.__enter__.return_value.get.return_value = mock_response
        mock_client_class.return_value = mock_client
        
        result = self.manager.check_health("ollama")
        
        self.assertFalse(result)
    
    @patch('httpx.Client')
    def test_health_check_network_error(self, mock_client_class):
        """Test health check with network error."""
        mock_client = MagicMock()
        mock_client.__enter__.return_value.get.side_effect = Exception("Network error")
        mock_client_class.return_value = mock_client
        
        result = self.manager.check_health("ollama")
        
        self.assertFalse(result)
    
    def test_health_check_nonexistent_service(self):
        """Test health check for non-existent service."""
        result = self.manager.check_health("nonexistent")
        
        self.assertFalse(result)


class TestServiceStartup(unittest.TestCase):
    """Test service startup and management."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.config = {
            "agentx": {"ollama_host": "localhost:11435"},
            "agentix": {"host": "localhost:8000"}
        }
        self.manager = ServiceManager(self.config)
    
    @patch('agentx.service_manager.ServiceManager.check_health')
    def test_start_service_already_running(self, mock_check):
        """Test starting service that's already running."""
        mock_check.return_value = True
        
        result = self.manager.start_service("ollama")
        
        self.assertTrue(result)
        # Should check health but not try to start
        mock_check.assert_called()
    
    @patch('agentx.service_manager.ServiceManager.check_health')
    @patch('subprocess.Popen')
    def test_start_service_success(self, mock_popen, mock_check):
        """Test successfully starting a service."""
        # First call returns False (not running), subsequent calls return True (started)
        mock_check.side_effect = [False, True]
        
        mock_process = Mock()
        mock_process.poll.return_value = None
        mock_popen.return_value = mock_process
        
        result = self.manager.start_service("ollama", timeout=5)
        
        self.assertTrue(result)
        mock_popen.assert_called_once()
    
    @patch('agentx.service_manager.ServiceManager.check_health')
    def test_start_service_timeout(self, mock_check):
        """Test service startup timeout."""
        mock_check.return_value = False
        
        result = self.manager.start_service("ollama", timeout=0.1)
        
        self.assertFalse(result)
    
    @patch('agentx.service_manager.ServiceManager.check_health')
    def test_start_service_no_command(self, mock_check):
        """Test starting service with no start command."""
        mock_check.return_value = False
        
        # Create a service without start command
        from agentx.service_manager import ServiceConfig
        service = ServiceConfig(
            name="NoStart",
            host="localhost",
            port=9999,
            health_endpoint="/health"
        )
        self.manager.services["nostart"] = service
        
        result = self.manager.start_service("nostart")
        
        self.assertFalse(result)


class TestEnsureServices(unittest.TestCase):
    """Test ensuring multiple services are running."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.config = {
            "agentx": {"ollama_host": "localhost:11435"},
            "agentix": {"host": "localhost:8000"}
        }
        self.manager = ServiceManager(self.config)
    
    @patch('agentx.service_manager.ServiceManager.start_service')
    def test_ensure_services_all_success(self, mock_start):
        """Test ensuring multiple services when all succeed."""
        mock_start.return_value = True
        
        result = self.manager.ensure_services(["ollama"])
        
        self.assertTrue(result)
        mock_start.assert_called()
    
    @patch('agentx.service_manager.ServiceManager.start_service')
    def test_ensure_services_partial_failure(self, mock_start):
        """Test ensuring multiple services when some fail."""
        mock_start.side_effect = [False, True]
        
        result = self.manager.ensure_services(["ollama", "agentix"])
        
        self.assertFalse(result)


class TestShutdown(unittest.TestCase):
    """Test service shutdown."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.config = {
            "agentx": {"ollama_host": "localhost:11435"},
            "agentix": {"host": "localhost:8000"}
        }
        self.manager = ServiceManager(self.config)
    
    def test_shutdown_running_process(self):
        """Test shutting down a running process."""
        mock_process = Mock()
        mock_process.poll.return_value = None  # Process is running
        
        self.manager._started_processes.append(mock_process)
        
        self.manager.shutdown()
        
        mock_process.terminate.assert_called_once()
        mock_process.wait.assert_called_once()
    
    def test_shutdown_already_stopped_process(self):
        """Test shutting down an already stopped process."""
        mock_process = Mock()
        mock_process.poll.return_value = 0  # Process already stopped
        
        self.manager._started_processes.append(mock_process)
        
        self.manager.shutdown()
        
        # Should not call terminate/wait if already stopped
        mock_process.terminate.assert_not_called()
    
    def test_shutdown_kill_timeout(self):
        """Test killing process that doesn't respond to terminate."""
        mock_process = Mock()
        mock_process.poll.return_value = None
        # First wait call (on terminate) raises TimeoutExpired, second wait call (after kill) succeeds
        mock_process.wait.side_effect = [subprocess.TimeoutExpired("cmd", 5), None]
        
        self.manager._started_processes.append(mock_process)
        
        # Should not raise, just log
        self.manager.shutdown()
        
        mock_process.terminate.assert_called_once()
        mock_process.kill.assert_called_once()


if __name__ == "__main__":
    print("=" * 70)
    print("TESTING SERVICE MANAGER")
    print("=" * 70)
    unittest.main(verbosity=2)
