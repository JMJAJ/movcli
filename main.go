package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Styling (Lipgloss) ---
var (
	baseStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF00D7")).
			Padding(1, 4)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")).
			Bold(true).
			MarginBottom(1)

	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00D7"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
)

// --- Models & Types ---
type sessionState int

const (
	stateSearch sessionState = iota
	stateLoading
	stateResults
	stateError
)

type item struct {
	title, desc, url string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type model struct {
	state       sessionState
	searchInput textinput.Model
	spinner     spinner.Model
	list        list.Model
	results     []list.Item
	err         error
	width       int // Track terminal width
	height      int // Track terminal height
}

type MovHubResponse struct {
	Status string `json:"status"`
	Result struct {
		Count int    `json:"count"`
		HTML  string `json:"html"`
	} `json:"result"`
}

// --- Init & Setup ---
func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Avengers, Joker, Matrix..."
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 40
	ti.PromptStyle = accentStyle
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = accentStyle

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Search Results"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Background(lipgloss.Color("#FF00D7")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)

	return model{
		state:       stateSearch,
		searchInput: ti,
		spinner:     sp,
		list:        l,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// --- Update Loop ---
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.state == stateResults {
				m.state = stateSearch
				m.searchInput.Focus()
				return m, nil
			}
		case "enter":
			if m.state == stateSearch && m.searchInput.Value() != "" {
				m.state = stateLoading
				query := m.searchInput.Value()
				return m, tea.Batch(m.spinner.Tick, fetchMoviesCmd(query))
			} else if m.state == stateResults {
				i, ok := m.list.SelectedItem().(item)
				if ok {
					openBrowser("https://movhub.ws" + i.url)
					return m, tea.Quit
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Cap the list width so it doesn't stretch too far on ultrawide monitors
		listWidth := msg.Width - 10
		if listWidth > 80 {
			listWidth = 80
		}
		m.list.SetSize(listWidth, msg.Height-6)

	case []list.Item:
		m.results = msg
		m.list.SetItems(msg)
		m.state = stateResults
		return m, nil

	case error:
		m.err = msg
		m.state = stateError
		return m, nil
	}

	switch m.state {
	case stateSearch:
		m.searchInput, cmd = m.searchInput.Update(msg)
	case stateLoading:
		m.spinner, cmd = m.spinner.Update(msg)
	case stateResults:
		m.list, cmd = m.list.Update(msg)
	}

	return m, cmd
}

// --- View Rendering ---
func (m model) View() string {
	// If dimensions aren't loaded yet, return empty
	if m.width == 0 {
		return ""
	}

	var content string

	switch m.state {
	case stateSearch:
		title := titleStyle.Render("🎬 MovCLI - Stream your favorites")
		input := m.searchInput.View()
		help := helpStyle.Render("Press ENTER to search • ESC to go back • CTRL+C to quit")

		uiBox := baseStyle.Render(fmt.Sprintf("%s\n\n%s\n\n%s", title, input, help))
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, uiBox)

	case stateLoading:
		loading := fmt.Sprintf("%s Searching for '%s'...", m.spinner.View(), m.searchInput.Value())
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, loading)

	case stateResults:
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.list.View())

	case stateError:
		errUI := fmt.Sprintf("Error: %v\nPress CTRL+C to exit.", m.err)
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, errUI)
	}

	return content
}

// --- HTTP Scraping Logic ---
func fetchMoviesCmd(query string) tea.Cmd {
	return func() tea.Msg {
		targetUrl := fmt.Sprintf("https://movhub.ws/ajax/film/search?keyword=%s", url.QueryEscape(query))
		req, err := http.NewRequest("GET", targetUrl, nil)
		if err != nil {
			return err
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var apiResp MovHubResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return err
		}

		reItem := regexp.MustCompile(`<a class="item" href="([^"]+)">.*?<span>([^<]+)</span>.*?<span>([^<]+)</span>.*?<span>([^<]+)</span>.*?<div class="title">([^<]+)</div>`)
		matches := reItem.FindAllStringSubmatch(apiResp.Result.HTML, -1)

		var items []list.Item
		for _, m := range matches {
			if len(m) == 6 {
				link := m[1]
				mediaType := m[2]
				yearOrSS := m[3]
				durationOrEp := m[4]
				title := m[5]

				desc := fmt.Sprintf("%s • %s • %s", mediaType, yearOrSS, durationOrEp)
				items = append(items, item{title: title, desc: desc, url: link})
			}
		}

		if len(items) == 0 {
			return fmt.Errorf("No results found for '%s'", query)
		}

		return items
	}
}

// --- Utilities ---
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Please open this link in your browser: %s\n", url)
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
