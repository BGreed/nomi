package term

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ollama/ollama/readline"
)

var (
	ErrInputInterrupted = errors.New("input interrupted")
	ErrInputKilled      = errors.New("input killed")
	ErrReadlineInit     = errors.New("error initializing readline")
)

func InitReadline() (*readline.Instance, error) {
	rl, err := readline.New(readline.Prompt{
		Prompt:      ">>> ",
		AltPrompt:   "...  ",
		Placeholder: "Send a message (/help for help)",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadlineInit, err)
	}
	return rl, nil
}

type MultilineState int

const (
	MultilineNone MultilineState = iota
	MultilinePrompt
	MultilineSystem
	MultilineTemplate
)

func readInput(rl *readline.Instance) (string, error) {
	var sb strings.Builder
	var multiline MultilineState

	for {
		line, err := rl.Readline()
		switch {
		case errors.Is(err, io.EOF):
			fmt.Println()
			return "", ErrInputKilled
		case errors.Is(err, readline.ErrInterrupt):
			if line == "" {
				fmt.Println("\nUse CTRL+D or /exit to exit.")
			}
			rl.Prompt.UseAlt = false
			sb.Reset()
			continue
		case err != nil:
			return "", fmt.Errorf("error reading input: %w", err)
		}

		switch {
		case multiline != MultilineNone:
			before, ok := strings.CutSuffix(line, `"""`)
			sb.WriteString(before)
			if !ok {
				fmt.Fprintln(&sb)
				continue
			}
			multiline = MultilineNone
			rl.Prompt.UseAlt = false
		case strings.HasPrefix(line, `"""`):
			line := strings.TrimPrefix(line, `"""`)
			line, ok := strings.CutSuffix(line, `"""`)
			sb.WriteString(line)
			if !ok {
				fmt.Fprintln(&sb)
				multiline = MultilinePrompt
				rl.Prompt.UseAlt = true
			}
			continue
		case rl.Pasting:
			fmt.Fprintln(&sb, line)
			continue
		default:
			sb.WriteString(line)
		}

		if sb.Len() > 0 && multiline == MultilineNone {
			return sb.String(), nil
		}
	}
}

func ReadInput(
	inputCh chan<- string,
	inputErrCh chan<- error,
	readyCh chan struct{},
) {
	defer close(readyCh)

	// Wait the readyCh once before starting the loop
	_, ok := <-readyCh
	if !ok {
		inputErrCh <- ErrInputInterrupted
		return
	}

	pipedInput, err := getPipedInput()
	if err != nil {
		inputErrCh <- fmt.Errorf("error reading piped input: %w", err)
		return
	}

	if pipedInput != "" {
		fmt.Println(">>>", pipedInput)
		inputCh <- pipedInput
	}

	rl, err := InitReadline()
	if err != nil {
		inputErrCh <- fmt.Errorf("%w: %v", ErrReadlineInit, err)
		return
	}

	fmt.Print(readline.StartBracketedPaste)
	defer fmt.Printf(readline.EndBracketedPaste)

	for {
		input, err := readInput(rl)
		if err != nil {
			inputErrCh <- err
			return
		}
		inputCh <- input

		// Wait for the readyCh before reading the next input
		_, ok = <-readyCh
		if !ok {
			inputErrCh <- ErrInputInterrupted
			return
		}
	}
}

func ReadInputOnce() (string, error) {
	rl, err := InitReadline()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrReadlineInit, err)
	}

	fmt.Print(readline.StartBracketedPaste)
	defer fmt.Printf(readline.EndBracketedPaste)

	return readInput(rl)
}
