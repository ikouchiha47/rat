package ui

import (
	"fmt"
	"strings"
	"time"

	"rat/internal/interfaces"
	"rat/internal/models"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model represents the main UI model
type Model struct {
	chatService interfaces.ChatService

	// UI components
	messagesViewport viewport.Model
	input            textinput.Model
	statusBar        viewport.Model

	// State
	messages    []*models.Message
	peers       []*models.Peer
	username    string
	currentRoom string

	// UI state
	ready        bool
	focus        FocusArea
	showHelp     bool
	windowWidth  int
	windowHeight int

	// Styles
	styles *Styles

	// Program reference for sending messages from goroutines
	program *tea.Program
}

// FocusArea represents which part of the UI has focus
type FocusArea int

const (
	FocusInput FocusArea = iota
	FocusMessages
	FocusPeers
)

// Styles contains all UI styles
type Styles struct {
	BorderColor lipgloss.Color
	InputColor  lipgloss.Color
	UserColor   lipgloss.Color
	SystemColor lipgloss.Color
	ErrorColor  lipgloss.Color
	PeerColor   lipgloss.Color
	RoomColor   lipgloss.Color

	BaseStyle    lipgloss.Style
	BorderStyle  lipgloss.Style
	InputStyle   lipgloss.Style
	MessageStyle lipgloss.Style
	SystemStyle  lipgloss.Style
	StatusStyle  lipgloss.Style
	HelpStyle    lipgloss.Style
}

// NewStyles creates new UI styles with vibrant colors
func NewStyles() *Styles {
	return &Styles{
		BorderColor: lipgloss.Color("#00D4AA"), // Bright teal
		InputColor:  lipgloss.Color("#FF6B9D"), // Bright pink
		UserColor:   lipgloss.Color("#00E676"), // Bright green
		SystemColor: lipgloss.Color("#FFB74D"), // Bright orange
		ErrorColor:  lipgloss.Color("#FF5252"), // Bright red
		PeerColor:   lipgloss.Color("#40C4FF"), // Bright blue
		RoomColor:   lipgloss.Color("#E040FB"), // Bright purple

		BaseStyle: lipgloss.NewStyle().
			Padding(0, 1).
			Margin(0),

		BorderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00D4AA")).
			Bold(true),

		InputStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF6B9D")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#1A1A2E")).
			Padding(0, 1).
			Bold(true),

		MessageStyle: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("#FFFFFF")),

		SystemStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB74D")).
			Bold(true).
			Italic(true),

		StatusStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("#16213E")).
			Foreground(lipgloss.Color("#00D4AA")).
			Padding(0, 1).
			Bold(true),

		HelpStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#40C4FF")).
			Background(lipgloss.Color("#0F3460")).
			Padding(1).
			Bold(true),
	}
}

// SetProgram sets the program reference for sending messages from goroutines
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// NewModel creates a new UI model
func NewModel(chatService interfaces.ChatService) *Model {
	styles := NewStyles()

	// Initialize input
	input := textinput.New()
	input.Placeholder = "Type a message..."
	input.Focus()
	input.CharLimit = 280
	input.Width = 50

	// Initialize viewports
	messagesViewport := viewport.New(80, 20)
	statusBar := viewport.New(80, 3)

	model := &Model{
		chatService:      chatService,
		messagesViewport: messagesViewport,
		input:            input,
		statusBar:        statusBar,
		username:         chatService.GetUsername(),
		currentRoom:      chatService.GetCurrentRoom(),
		focus:            FocusInput,
		styles:           styles,
	}

	// Set up event handler for chat service
	chatService.SetEventHandler(&chatEventHandler{model: model})

	return model
}

// Init returns initial commands for Bubble Tea
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.refreshMessages,
		m.refreshPeers,
		m.tickPeersRefresh, // Start periodic peer refresh
	)
}

// tickPeersRefresh creates a periodic refresh for peers list
func (m *Model) tickPeersRefresh() tea.Msg {
	return tea.Tick(time.Second*5, func(t time.Time) tea.Msg {
		return refreshPeersMsg{}
	})()
}

// Update handles Bubble Tea update messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.handleResize(msg)
		m.ready = true

	case tea.KeyMsg:
		// Handle global keybindings but do not return early so the input
		// component still receives key events for typing.
		if _, cmd := m.handleKeyMsg(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case refreshMessagesMsg:
		m.refreshMessagesContent()

	case refreshPeersMsg:
		m.refreshPeersContent()
		// Also refresh UI state (room, username) in case they changed
		m.currentRoom = m.chatService.GetCurrentRoom()
		m.username = m.chatService.GetUsername()
		// Schedule next refresh
		cmds = append(cmds, m.tickPeersRefresh)

	case roomChangedMsg:
		// Update current room and refresh UI
		m.currentRoom = msg.roomName
		m.refreshMessagesContent()
		m.refreshPeersContent()

	case errorMsg:
		// Handle error messages
		m.addSystemMessage(fmt.Sprintf("Error: %s", msg.err), true)
	}

	// Update input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Update viewports
	m.messagesViewport, cmd = m.messagesViewport.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	m.statusBar, cmd = m.statusBar.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the current UI state
func (m *Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	// Build the UI layout
	messagesView := m.renderMessages()
	inputView := m.renderInput()
	statusView := m.renderStatusBar()
	peersView := m.renderPeers()

	// Combine views
	leftPane := lipgloss.JoinVertical(
		lipgloss.Left,
		messagesView,
		inputView,
	)

	mainView := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPane,
		peersView,
	)

	fullView := lipgloss.JoinVertical(
		lipgloss.Left,
		mainView,
		statusView,
	)

	if m.showHelp {
		fullView = lipgloss.JoinVertical(
			lipgloss.Left,
			fullView,
			m.renderHelp(),
		)
	}

	return fullView
}

// handleKeyMsg handles keyboard input
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "enter":
		return m.handleSendMessage()

	case "tab":
		m.cycleFocus()
		return m, nil

	case "?":
		m.showHelp = !m.showHelp
		return m, nil

	case "ctrl+r":
		return m, m.refreshMessages

	case "ctrl+p":
		return m, m.refreshPeers
	}

	return m, nil
}

// handleSendMessage sends the current input as a message
func (m *Model) handleSendMessage() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input.Value())
	if input == "" {
		return m, nil
	}

	// Handle commands
	if strings.HasPrefix(input, "/") {
		return m.handleCommand(input)
	}

	// Send regular message
	err := m.chatService.SendMessage(input, "")
	if err != nil {
		return m, func() tea.Msg { return errorMsg{err} }
	}

	m.input.Reset()
	return m, m.refreshMessages
}

// handleCommand handles slash commands
func (m *Model) handleCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(input, " ", 2)
	command := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	switch command {
	case "/register":
		if args != "" {
			m.addSystemMessage(fmt.Sprintf("Attempting to register as: %s", args), false)
			err := m.chatService.RegisterUser(args)
			if err != nil {
				m.addSystemMessage(fmt.Sprintf("Registration failed: %s", err.Error()), true)
				return m, func() tea.Msg { return errorMsg{err} }
			}
			m.username = args
			m.addSystemMessage(fmt.Sprintf("Successfully registered as: %s", args), false)
		} else {
			m.addSystemMessage("Usage: /register <username>", true)
		}

	case "/connect":
		if args != "" {
			m.addSystemMessage(fmt.Sprintf("Attempting to connect to room: %s", args), false)
			err := m.chatService.ConnectRoom(args)
			if err != nil {
				m.addSystemMessage(fmt.Sprintf("Failed to connect: %s", err.Error()), true)
				return m, func() tea.Msg { return errorMsg{err} }
			}
			m.currentRoom = args
			m.addSystemMessage(fmt.Sprintf("Successfully connected to room: %s", args), false)

			// Refresh users in room
			users := m.chatService.GetUsersInCurrentRoom()
			if len(users) > 0 {
				m.addSystemMessage(fmt.Sprintf("Users in room: %s", strings.Join(users, ", ")), false)
			} else {
				m.addSystemMessage("You are the first user in this room", false)
			}
		} else {
			m.addSystemMessage("Usage: /connect <room>", true)
		}

	case "/leave":
		err := m.chatService.LeaveRoom()
		if err != nil {
			return m, func() tea.Msg { return errorMsg{err} }
		}
		m.currentRoom = ""
		m.addSystemMessage("Left current room", false)

	case "/room":
		if m.currentRoom == "" {
			m.addSystemMessage("Not connected to any room", false)
		} else {
			m.addSystemMessage(fmt.Sprintf("Current room: %s", m.currentRoom), false)
		}

	case "/users":
		if m.currentRoom == "" {
			m.addSystemMessage("Not connected to any room", false)
		} else {
			users := m.chatService.GetUsersInCurrentRoom()
			if len(users) == 0 {
				m.addSystemMessage("No users in current room", false)
			} else {
				m.addSystemMessage(fmt.Sprintf("Users in room: %s", strings.Join(users, ", ")), false)
			}
		}

	case "/rooms":
		rooms := m.chatService.GetAllRooms()
		if len(rooms) == 0 {
			m.addSystemMessage("No active rooms", false)
		} else {
			m.addSystemMessage(fmt.Sprintf("Active rooms: %s", strings.Join(rooms, ", ")), false)
		}

	case "/help":
		m.showHelp = !m.showHelp

	default:
		m.addSystemMessage(fmt.Sprintf("Unknown command: %s", command), true)
	}

	m.input.Reset()
	return m, nil
}

// cycleFocus cycles between different focus areas
func (m *Model) cycleFocus() {
	switch m.focus {
	case FocusInput:
		m.focus = FocusMessages
		m.input.Blur()
	case FocusMessages:
		m.focus = FocusPeers
	case FocusPeers:
		m.focus = FocusInput
		m.input.Focus()
	}
}

// refreshMessagesContent refreshes the messages viewport
func (m *Model) refreshMessagesContent() {
	messages := m.chatService.GetMessages()
	m.messages = messages

	var content strings.Builder
	for _, msg := range messages {
		content.WriteString(m.formatMessage(msg))
		content.WriteString("\n")
	}

	m.messagesViewport.SetContent(content.String())
	m.messagesViewport.GotoBottom()
}

// refreshPeersContent refreshes the peers list
func (m *Model) refreshPeersContent() {
	m.peers = m.chatService.GetPeers()
}

// formatMessage formats a message for display
func (m *Model) formatMessage(msg *models.Message) string {
	if msg.IsSystemMessage() {
		return m.styles.SystemStyle.Render(
			fmt.Sprintf("[%s] %s", msg.Timestamp.Format("15:04"), msg.Content),
		)
	}

	if msg.IsPrivate() {
		return m.styles.MessageStyle.Render(
			fmt.Sprintf("[%s] %s → %s: %s",
				msg.Timestamp.Format("15:04"),
				msg.From,
				msg.To,
				msg.Content),
		)
	}

	return m.styles.MessageStyle.Render(
		fmt.Sprintf("[%s] %s: %s",
			msg.Timestamp.Format("15:04"),
			msg.From,
			msg.Content),
	)
}

// addSystemMessage adds a system message to the chat
func (m *Model) addSystemMessage(content string, isError bool) {
	msg := &models.Message{
		Type:      models.MessageTypeChat,
		From:      "system",
		Content:   content,
		Timestamp: time.Now(),
		Room:      m.currentRoom,
	}

	m.messages = append(m.messages, msg)
	m.refreshMessagesContent()
}

// renderMessages renders the messages viewport
func (m *Model) renderMessages() string {
	if !m.ready {
		return "Loading messages..."
	}

	return m.styles.BorderStyle.
		Width(m.windowWidth * 3 / 4).
		Height(m.windowHeight - 6).
		Render(m.messagesViewport.View())
}

// renderInput renders the input field
func (m *Model) renderInput() string {
	if !m.ready {
		return ""
	}

	input := m.styles.InputStyle.
		Width(m.windowWidth*3/4 - 2).
		Render(m.input.View())

	return input
}

// renderPeers renders the peers list with vibrant colors
func (m *Model) renderPeers() string {
	if !m.ready {
		return "Loading peers..."
	}

	// Get fresh peer data from chat service
	peers := m.chatService.GetPeers()
	
	var content strings.Builder
	
	// Title with vibrant styling
	titleStyle := lipgloss.NewStyle().
		Foreground(m.styles.PeerColor).
		Bold(true).
		Underline(true)
	content.WriteString(titleStyle.Render("🌐 Online Users") + "\n\n")

	if len(peers) == 0 {
		noUsersStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF9800")).
			Italic(true)
		content.WriteString(noUsersStyle.Render("No users online"))
	} else {
		for _, peer := range peers {
			// Only show peers in the same room or if we're not in a room yet
			if m.currentRoom == "" || peer.Room == m.currentRoom {
				status := "🟢" // Online
				if !peer.IsConnected {
					status = "🔴" // Offline
				}
				
				peerStyle := lipgloss.NewStyle().
					Foreground(m.styles.PeerColor).
					Bold(true)
				roomStyle := lipgloss.NewStyle().
					Foreground(m.styles.RoomColor).
					Italic(true)
					
				content.WriteString(fmt.Sprintf("%s %s %s\n",
					status,
					peerStyle.Render(peer.Username),
					roomStyle.Render("("+peer.Room+")")))
			}
		}
	}

	// Vibrant border for peers panel
	peersStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.PeerColor).
		Width(m.windowWidth / 4).
		Height(m.windowHeight - 3).
		Padding(1).
		Bold(true)

	return peersStyle.Render(content.String())
}

// renderStatusBar renders the status bar
func (m *Model) renderStatusBar() string {
	if !m.ready {
		return ""
	}

	var status string
	if m.currentRoom == "" {
		if m.username == "" {
			status = "Status: Not registered | Press ? for help"
		} else {
			status = fmt.Sprintf("User: %s | Status: Not in room | Press ? for help", m.username)
		}
	} else {
		users := m.chatService.GetUsersInCurrentRoom()
		status = fmt.Sprintf("User: %s | Room: %s | Users: %d | Press ? for help",
			m.username,
			m.currentRoom,
			len(users))
	}

	return m.styles.StatusStyle.
		Width(m.windowWidth).
		Render(status)
}

// renderHelp renders the help text
func (m *Model) renderHelp() string {
	help := `🚀 Chat Commands:
  /register <username> - Register with a username
  /connect <room>      - Connect to a room
  /leave               - Leave current room
  /room                - Show current room
  /users               - Show users in current room
  /rooms               - Show all available rooms
  /help                - Show this help
  
💬 Messaging:
  @username message   - Send private message to user
  regular message     - Send message to room
  
⌨️  Keys:
  Enter               - Send message
  Tab                 - Cycle focus
  ?                   - Toggle help
  Ctrl+C, q           - Quit`

	return m.styles.HelpStyle.
		Width(m.windowWidth - 4).
		Render(help)
}

// refreshMessages returns a command to refresh messages
func (m *Model) refreshMessages() tea.Msg {
	return refreshMessagesMsg{}
}

// refreshPeers returns a command to refresh peers
func (m *Model) refreshPeers() tea.Msg {
	return refreshPeersMsg{}
}

// handleResize handles window resize events
func (m *Model) handleResize(msg tea.WindowSizeMsg) {
	// Update viewport dimensions
	m.messagesViewport.Width = msg.Width - 30  // Leave space for peers panel
	m.messagesViewport.Height = msg.Height - 6 // Leave space for input and status

	m.input.Width = msg.Width - 32

	m.statusBar.Width = msg.Width - 2
	m.statusBar.Height = 3
}

// Message types for Bubble Tea
type refreshMessagesMsg struct{}
type refreshPeersMsg struct{}
type errorMsg struct{ err error }
type roomChangedMsg struct{ roomName string }

// chatEventHandler implements ChatEventHandler for UI updates
type chatEventHandler struct {
	model *Model
}

func (h *chatEventHandler) OnMessageReceived(message *models.Message) {
	// Schedule a refresh of the messages view
	// message can be nil to indicate a general refresh
	go func() {
		// Use a goroutine to avoid blocking the event handler
		// This is safe because it just schedules a command via the Bubble Tea runtime
		if h.model.program != nil {
			h.model.program.Send(refreshMessagesMsg{})
		}
	}()
}

func (h *chatEventHandler) OnPeerJoined(peer *models.Peer) {
	// Schedule a refresh of the peers view
	go func() {
		if h.model.program != nil {
			h.model.program.Send(refreshPeersMsg{})
		}
	}()
}

func (h *chatEventHandler) OnPeerLeft(peerID string) {
	// Schedule a refresh of the peers view
	go func() {
		if h.model.program != nil {
			h.model.program.Send(refreshPeersMsg{})
		}
	}()
}

func (h *chatEventHandler) OnRoomChanged(roomName string) {
	// Update current room and schedule a refresh of the peers view
	go func() {
		if h.model.program != nil {
			h.model.program.Send(roomChangedMsg{roomName: roomName})
		}
	}()
}

func (h *chatEventHandler) OnError(err error) {
	// Schedule an error message
	go func() {
		if h.model.program != nil {
			h.model.program.Send(errorMsg{err: err})
		}
	}()
}
