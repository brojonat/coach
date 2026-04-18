package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"coach/internal/lab"
)

const defaultDBPath = "coach.db"

func labCommand() *cli.Command {
	return &cli.Command{
		Name:  "lab",
		Usage: "prompt-optimization lab (tasks, experiments, ratings)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Value: defaultDBPath, Usage: "path to the lab SQLite database"},
		},
		Commands: []*cli.Command{
			{
				Name:  "tasks",
				Usage: "manage the task corpus",
				Commands: []*cli.Command{
					{
						Name:   "seed",
						Usage:  "insert the starter task corpus (idempotent)",
						Action: tasksSeedAction,
					},
					{
						Name:  "list",
						Usage: "list tasks in the corpus",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "tag", Usage: "filter by tag (e.g. git, files, dangerous)"},
						},
						Action: tasksListAction,
					},
					{
						Name:      "show",
						Usage:     "show full details for a single task",
						ArgsUsage: "<slug>",
						Action:    tasksShowAction,
					},
				},
			},
		},
	}
}

// openStore honors the --db flag from the nearest ancestor (the `lab` cmd).
func openStore(ctx context.Context, c *cli.Command) (*lab.Store, error) {
	path := c.String("db")
	if path == "" {
		path = defaultDBPath
	}
	return lab.Open(ctx, path)
}

func tasksSeedAction(ctx context.Context, c *cli.Command) error {
	store, err := openStore(ctx, c)
	if err != nil {
		return err
	}
	defer store.Close()

	inserted, skipped, err := store.SeedTasks(ctx)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "seeded %d new tasks (%d already present)\n", inserted, skipped)
	return nil
}

func tasksListAction(ctx context.Context, c *cli.Command) error {
	store, err := openStore(ctx, c)
	if err != nil {
		return err
	}
	defer store.Close()

	tasks, err := store.ListTasks(ctx, c.String("tag"))
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	if len(tasks) == 0 {
		fmt.Fprintln(os.Stderr, "no tasks — run `coach lab tasks seed` to populate the corpus")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SLUG\tDIFFICULTY\tTAGS\tTITLE")
	for _, t := range tasks {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.Slug, t.Difficulty, strings.Join(t.Tags, ","), t.Title)
	}
	return tw.Flush()
}

func tasksShowAction(ctx context.Context, c *cli.Command) error {
	slug := c.Args().First()
	if slug == "" {
		return errors.New("usage: coach lab tasks show <slug>")
	}
	store, err := openStore(ctx, c)
	if err != nil {
		return err
	}
	defer store.Close()

	t, err := store.GetTask(ctx, slug)
	if errors.Is(err, lab.ErrTaskNotFound) {
		return fmt.Errorf("no task with slug %q (try `coach lab tasks list`)", slug)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "slug:       %s\n", t.Slug)
	fmt.Fprintf(os.Stdout, "title:      %s\n", t.Title)
	fmt.Fprintf(os.Stdout, "difficulty: %s\n", t.Difficulty)
	fmt.Fprintf(os.Stdout, "tags:       %s\n", strings.Join(t.Tags, ", "))
	fmt.Fprintf(os.Stdout, "created:    %s\n", t.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(os.Stdout, "\ngoal:\n  %s\n", t.Goal)
	if t.Notes != "" {
		fmt.Fprintf(os.Stdout, "\nnotes:\n  %s\n", t.Notes)
	}
	return nil
}
