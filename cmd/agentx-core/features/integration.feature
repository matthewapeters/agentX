@integration
Feature: IPC router integration
  As an AgentX core maintainer
  I want IPC FIFO paths to be provisioned hermetically
  So that Python applets can communicate with core reliably

  Scenario: FIFO pair is created for an applet
    Given a temporary project directory
    And an IPC router for session "sess-1"
    When I create an IPC FIFO pair for applet "chat"
    Then both FIFO files should exist
    And FIFO paths should include applet name "chat"
