package output

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/charmbracelet/glamour"
	"github.com/mattn/go-isatty"
)

// getPagerPath finds pager path using PAGER env variable or fallback options
func getPagerPath(fallback ...string) string {
	p := os.Getenv("PAGER")
	if p != "" {
		if p == "NOPAGER" {
			return ""
		}

		exe, err := exec.LookPath(p)
		if err != nil {
			// If PAGER is set but not found, fall back to default behavior
			// instead of panicking like go-pager does
		} else {
			return exe
		}
	}

	// Try fallback options
	for _, pager := range fallback {
		if exe, err := exec.LookPath(pager); err == nil {
			return exe
		}
	}

	return ""
}

// NewRenderer creates a standardized glamour renderer for raft commands
func NewRenderer() (*glamour.TermRenderer, error) {
	return glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(160),
	)
}

// RenderMarkdown renders markdown content using the standardized renderer with optional pager support
func RenderMarkdown(md string, noPager bool) error {
	r, err := NewRenderer()
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	out, err := r.Render(md)
	if err != nil {
		return fmt.Errorf("failed to render output: %w", err)
	}

	// Check if we should use a pager
	if !noPager && isatty.IsTerminal(os.Stdout.Fd()) {
		return pipeToPager(out)
	}

	fmt.Print(out)
	return nil
}

// pipeToPager pipes the output through a pager, respecting $PAGER environment variable
func pipeToPager(content string) error {
	pagerPath := getPagerPath("less", "more")
	if pagerPath == "" {
		// No pager available or NOPAGER set, output directly
		fmt.Print(content)
		return nil
	}

	var pagerCmd *exec.Cmd
	if pagerPath == "less" {
		pagerCmd = exec.CommandContext(context.Background(), pagerPath, "-R") //nolint:gosec
	} else {
		pagerCmd = exec.CommandContext(context.Background(), pagerPath) //nolint:gosec
	}

	// Set up pipes
	stdin, err := pagerCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create pager stdin pipe: %w", err)
	}

	pagerCmd.Stdout = os.Stdout
	pagerCmd.Stderr = os.Stderr

	// Start the pager
	if err := pagerCmd.Start(); err != nil {
		// If pager fails to start, output directly
		fmt.Print(content)
		return nil
	}

	// Write content to pager
	_, err = io.WriteString(stdin, content)
	if err != nil {
		stdin.Close() //nolint:errcheck
		return fmt.Errorf("failed to write to pager: %w", err)
	}

	// Close stdin to signal EOF
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("failed to close pager stdin: %w", err)
	}

	// Wait for pager to finish
	return pagerCmd.Wait()
}
