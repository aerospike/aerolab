package printer

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

var ErrTerminalWidthUnknown = errors.New("terminal width unknown")

type TableWriter struct {
	t              table.Writer
	r              func() string
	ColorHiWhite   colorPrint
	ColorWarn      colorPrint
	ColorErr       colorPrint
	IsColorEnabled bool
	IsTerminal     bool
}

func GetTableWriter(renderType string, theme string, sortBy []string) (*TableWriter, error) {
	// sortBy parsing
	sort := []table.SortBy{}
	for _, sortItem := range sortBy {
		sortSplit := strings.Split(sortItem, ":")
		if len(sortSplit) != 2 {
			return nil, errors.New("sort item wrong format")
		}
		mmode := table.Asc
		switch sortSplit[1] {
		case "asc":
			mmode = table.Asc
		case "dsc":
			mmode = table.Dsc
		case "ascnum":
			mmode = table.AscNumeric
		case "dscnum":
			mmode = table.DscNumeric
		default:
			return nil, errors.New("sort item incorrect modifier")
		}
		sort = append(sort, table.SortBy{
			Name: sortSplit[0],
			Mode: mmode,
		})
	}

	// create the table writer
	t := table.NewWriter()

	// sort out renderer type
	type renderer func() string
	var render renderer = t.Render
	switch strings.ToUpper(renderType) {
	case "HTML":
		render = t.RenderHTML
	case "CSV":
		render = t.RenderCSV
	case "TSV":
		render = t.RenderTSV
	case "MARKDOWN":
		render = t.RenderMarkdown
	}

	// define basic colors
	hiWhiteColor := colorPrint{c: text.Colors{text.FgHiWhite}, enable: true}
	warnColor := colorPrint{c: text.Colors{text.BgHiYellow, text.FgBlack}, enable: true}
	errColor := colorPrint{c: text.Colors{text.BgHiRed, text.FgWhite}, enable: true}

	// test if we are in a real terminal
	isTerminal := false
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		isTerminal = true
	}

	// figure out if colors are to be enabled
	isColor := isTerminal
	if theme != "default" {
		isColor = false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok || os.Getenv("CLICOLOR") == "0" {
		isColor = false
	}

	// enable sorting if requested
	if len(sortBy) > 0 {
		t.SortBy(sort)
	}

	// set style
	if !isColor {
		t.SetStyle(table.StyleDefault)
		hiWhiteColor.enable = false
		warnColor.enable = false
		errColor.enable = false
		switch theme {
		case "frame":
			t.SetStyle(table.StyleRounded)
			tstyle := t.Style()
			tstyle.Options.DrawBorder = true
			tstyle.Options.SeparateColumns = false
		case "box":
			t.SetStyle(table.StyleRounded)
			tstyle := t.Style()
			tstyle.Options.SeparateColumns = true
		default:
			tstyle := t.Style()
			tstyle.Options.DrawBorder = false
			tstyle.Options.SeparateColumns = false
		}
	} else {
		t.SetStyle(table.StyleColoredBlackOnCyanWhite)
	}

	// set width
	if isTerminal {
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width < 1 {
			return nil, ErrTerminalWidthUnknown
		} else {
			if width < 40 {
				width = 40
			}
			t.SetAllowedRowLength(width)
		}
	}

	// set header and footer defaults
	tstyle := t.Style()
	tstyle.Format.Header = text.FormatDefault
	tstyle.Format.Footer = text.FormatDefault
	return &TableWriter{t: t, r: render, ColorHiWhite: hiWhiteColor, ColorWarn: warnColor, ColorErr: errColor, IsColorEnabled: isColor, IsTerminal: isTerminal}, nil
}

func (t *TableWriter) RenderTable(title *string, header table.Row, rows []table.Row) string {
	if title != nil {
		t.t.SetTitle(t.ColorHiWhite.Sprint(*title))
	}
	t.t.AppendHeader(header)
	for _, row := range rows {
		t.t.AppendRow(row)
	}
	return t.r()
}

type colorPrint struct {
	c      text.Colors
	enable bool
}

func (c *colorPrint) Sprint(a ...interface{}) string {
	if c.enable {
		return c.c.Sprint(a...)
	}
	return fmt.Sprint(a...)
}

func (c *colorPrint) Sprintf(format string, a ...interface{}) string {
	if c.enable {
		return c.c.Sprintf(format, a...)
	}
	return fmt.Sprintf(format, a...)
}

func String(s string) *string {
	return &s
}
