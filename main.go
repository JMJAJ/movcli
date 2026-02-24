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
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// -- Palette ------------------------------------------------------------------
var (
	clrYellow = lipgloss.Color("#F5E642")
	clrWhite  = lipgloss.Color("#EEEEEE")
	clrGray   = lipgloss.Color("#888888")
	clrDark   = lipgloss.Color("#444444")
	clrBlack  = lipgloss.Color("#111111")
)

// -- Styles -------------------------------------------------------------------
var (
	outerStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(clrWhite).
			Padding(1, 3).
			Width(58)

	logoStyle = lipgloss.NewStyle().
			Foreground(clrYellow).
			Bold(true)

	subStyle = lipgloss.NewStyle().
			Foreground(clrGray)

	labelStyle = lipgloss.NewStyle().
			Foreground(clrYellow).
			Bold(true)

	divStyle = lipgloss.NewStyle().
			Foreground(clrDark)

	hintStyle = lipgloss.NewStyle().
			Foreground(clrGray)

	keyStyle = lipgloss.NewStyle().
			Foreground(clrBlack).
			Background(clrYellow).
			Bold(true).
			Padding(0, 1)

	loadStyle = lipgloss.NewStyle().
			Foreground(clrWhite).
			Bold(true)

	errStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(clrYellow).
			Foreground(clrWhite).
			Padding(1, 3)

	selectedTitleStyle = lipgloss.NewStyle().
				Foreground(clrYellow).
				Bold(true)

	normalTitleStyle = lipgloss.NewStyle().
				Foreground(clrWhite)

	selectedDescStyle = lipgloss.NewStyle().
				Foreground(clrGray)

	normalDescStyle = lipgloss.NewStyle().
			Foreground(clrDark)

	listHeaderStyle = lipgloss.NewStyle().
			Foreground(clrBlack).
			Background(clrYellow).
			Bold(true).
			Padding(0, 2)

	countStyle = lipgloss.NewStyle().
			Foreground(clrGray)
)

// -- Session state ------------------------------------------------------------
type sessionState int

const (
	stateSearch sessionState = iota
	stateLoading
	stateResults
	stateError
)

// -- List item ----------------------------------------------------------------
type item struct {
	title, desc, itemURL string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// -- Custom delegate ----------------------------------------------------------
type delegate struct{}

func (d delegate) Height() int                             { return 2 }
func (d delegate) Spacing() int                            { return 1 }
func (d delegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d delegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}
	sel := index == m.Index()
	prefix := "  "
	titleS := normalTitleStyle.Render(i.title)
	descS := normalDescStyle.Render(i.desc)
	if sel {
		prefix = selectedTitleStyle.Render("> ")
		titleS = selectedTitleStyle.Render(i.title)
		descS = selectedDescStyle.Render(i.desc)
	}
	fmt.Fprintf(w, "%s%s\n  %s", prefix, titleS, descS)
}

// -- App model ----------------------------------------------------------------
type model struct {
	state       sessionState
	searchInput textinput.Model
	spinner     spinner.Model
	list        list.Model
	resultCount int
	err         error
	width       int
	height      int
}

type MovHubResponse struct {
	Status string `json:"status"`
	Result struct {
		Count int    `json:"count"`
		HTML  string `json:"html"`
	} `json:"result"`
}

// -- Init ---------------------------------------------------------------------
func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "search title..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 44
	ti.PromptStyle = lipgloss.NewStyle().Foreground(clrYellow).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(clrWhite)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(clrYellow)

	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = lipgloss.NewStyle().Foreground(clrYellow)

	l := list.New([]list.Item{}, delegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.InfiniteScrolling = true

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

// -- Update -------------------------------------------------------------------
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.state != stateResults || !m.list.SettingFilter() {
				return m, tea.Quit
			}
		case "esc":
			if m.state == stateResults || m.state == stateError {
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
					openBrowser("https://movhub.ws" + i.itemURL)
					return m, tea.Quit
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listWidth := msg.Width - 8
		if listWidth > 84 {
			listWidth = 84
		}
		m.list.SetSize(listWidth, msg.Height-7)

	case []list.Item:
		m.resultCount = len(msg)
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

// -- Views --------------------------------------------------------------------
func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.state {
	case stateSearch:
		return m.viewSearch()
	case stateLoading:
		return m.viewLoading()
	case stateResults:
		return m.viewResults()
	case stateError:
		return m.viewError()
	}
	return ""
}

func (m model) viewSearch() string {
	logo := logoStyle.Render("MOVCLI")
	sub := subStyle.Render("stream anything from your terminal")
	div := divStyle.Render(strings.Repeat("-", 50))
	label := labelStyle.Render("SEARCH")
	field := "  " + m.searchInput.View()

	enter := keyStyle.Render("ENTER")
	quit := keyStyle.Render("CTRL+C")
	hint := hintStyle.Render(fmt.Sprintf("  %s search   %s quit", enter, quit))

	inner := strings.Join([]string{logo, sub, "", div, "", label, field, "", hint}, "\n")
	box := outerStyle.Render(inner)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) viewLoading() string {
	line := loadStyle.Render(fmt.Sprintf("  %s  searching for \"%s\"", m.spinner.View(), m.searchInput.Value()))
	box := outerStyle.Render("\n" + line + "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) viewResults() string {
	header := listHeaderStyle.Render("RESULTS")
	count := countStyle.Render(fmt.Sprintf("  %d results for \"%s\"", m.resultCount, m.searchInput.Value()))
	headerRow := lipgloss.JoinHorizontal(lipgloss.Center, header, count)

	div := divStyle.Render(strings.Repeat("-", m.list.Width()))

	updown := keyStyle.Render("UP/DOWN")
	enter := keyStyle.Render("ENTER")
	esc := keyStyle.Render("ESC")
	slash := keyStyle.Render("/")
	hints := hintStyle.Render(fmt.Sprintf("  %s navigate   %s open   %s back   %s filter", updown, enter, esc, slash))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left,
			headerRow,
			div,
			m.list.View(),
			div,
			hints,
		),
	)
}

func (m model) viewError() string {
	label := labelStyle.Render("ERROR")
	msg := lipgloss.NewStyle().Foreground(clrWhite).Render(m.err.Error())
	hint := hintStyle.Render("press ESC to go back")
	box := errStyle.Render(strings.Join([]string{label, "", msg, "", hint}, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// -- HTTP fetch ---------------------------------------------------------------
func fetchMoviesCmd(query string) tea.Cmd {
	return func() tea.Msg {
		targetURL := fmt.Sprintf("https://movhub.ws/ajax/film/search?keyword=%s", url.QueryEscape(query))
		req, err := http.NewRequest("GET", targetURL, nil)
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

		re := regexp.MustCompile(`<a class="item" href="([^"]+)">.*?<span>([^<]+)</span>.*?<span>([^<]+)</span>.*?<span>([^<]+)</span>.*?<div class="title">([^<]+)</div>`)
		matches := re.FindAllStringSubmatch(apiResp.Result.HTML, -1)

		var items []list.Item
		for _, m := range matches {
			if len(m) == 6 {
				desc := fmt.Sprintf("%s  %s  %s", m[2], m[3], m[4])
				items = append(items, item{title: m[5], desc: desc, itemURL: m[1]})
			}
		}

		if len(items) == 0 {
			return fmt.Errorf("no results for %q", query)
		}
		return items
	}
}

// -- Utilities ----------------------------------------------------------------
func openBrowser(u string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		if err = exec.Command("xdg-open", u).Start(); err != nil {
			err = exec.Command("am", "start", "--user", "0",
				"-a", "android.intent.action.VIEW",
				"-d", u).Start()
		}
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		err = exec.Command("open", u).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Open in browser: %s\n", u)
	}
}

func selfCleanup() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Remove(exe)
	}()
}

func main() {
	defer selfCleanup()

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
