// Command flat-email is the v0.1 CLI: it imports local mail stores (mbox files
// and Maildir directories) into a deterministic, SPEC.md-conformant archive.
//
// Network connectors (Gmail/Outlook/IMAP), search, serve, and mcp are documented
// in the README as the target interface but are out of scope for v0.1.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/japer-technology/flat-email/internal/archive"
	"github.com/japer-technology/flat-email/internal/mailstore"
	"github.com/japer-technology/flat-email/internal/model"
	"github.com/japer-technology/flat-email/internal/storage"
)

const usage = `flat-email — portable email archiving (v0.1, local import only)

Usage:
  flat-email import --account <email> --out <dir> [--mbox <file>]... [--maildir <dir>]... [--label <name>]

Flags:
  --account   Account email address the imported mail belongs to (required).
  --out       Archive output directory (required).
  --mbox      An mbox file to import (repeatable).
  --maildir   A Maildir directory to import (repeatable).
  --label     Override the label applied to imported messages (defaults to the
              source file/dir name).
  --created-by  Value recorded in the archive manifest's createdBy field.

Example:
  flat-email import --account me@example.com --mbox inbox.mbox --out ./my-archive
`

type stringList []string

func (s *stringList) String() string { return fmt.Sprint(*s) }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "import":
		if err := runImport(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	var (
		account   = fs.String("account", "", "account email address")
		out       = fs.String("out", "", "archive output directory")
		label     = fs.String("label", "", "label override for imported messages")
		createdBy = fs.String("created-by", "", "manifest createdBy value")
		mboxes    stringList
		maildirs  stringList
	)
	fs.Var(&mboxes, "mbox", "mbox file to import (repeatable)")
	fs.Var(&maildirs, "maildir", "Maildir directory to import (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *account == "" {
		return fmt.Errorf("--account is required")
	}
	if *out == "" {
		return fmt.Errorf("--out is required")
	}
	if len(mboxes) == 0 && len(maildirs) == 0 {
		return fmt.Errorf("provide at least one --mbox or --maildir source")
	}

	var messages []model.Message
	for _, m := range mboxes {
		ms, err := mailstore.ReadMboxFile(m, *label)
		if err != nil {
			return fmt.Errorf("read mbox %s: %w", m, err)
		}
		messages = append(messages, ms...)
	}
	for _, d := range maildirs {
		ms, err := mailstore.ReadMaildir(d, *label)
		if err != nil {
			return fmt.Errorf("read maildir %s: %w", d, err)
		}
		messages = append(messages, ms...)
	}

	in := model.Input{
		SyncTime:  time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
		CreatedBy: *createdBy,
		Accounts: []model.Account{
			{Address: *account, Messages: messages},
		},
	}

	if err := archive.Produce(storage.NewFS(*out), in); err != nil {
		return err
	}
	fmt.Printf("imported %d message(s) into %s\n", len(messages), filepath.Clean(*out))
	return nil
}
