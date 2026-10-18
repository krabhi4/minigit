package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"minigit/pkg/object"
	"minigit/pkg/repo"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = handleInit(args)
	case "hash-object":
		err = handleHashObject(args)
	case "cat-file":
		err = handleCatFile(args)
	case "add":
		err = handleAdd(args)
	case "rm":
		err = handleRm(args)
	case "write-tree":
		err = handleWriteTree(args)
	case "commit":
		err = handleCommit(args)
	case "status":
		err = handleStatus(args)
	case "log":
		err = handleLog(args)
	case "branch":
		err = handleBranch(args)
	case "checkout":
		err = handleCheckout(args)
	case "diff":
		err = handleDiff(args)
	case "tag":
		err = handleTag(args)
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "minigit: '%s' is not a minigit command. See 'minigit --help'.\n", cmd)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("usage: minigit <command> [<args>]")
	fmt.Println()
	fmt.Println("Available commands:")
	fmt.Println("   init         Create an empty Git repository")
	fmt.Println("   hash-object  Compute object ID and optionally create an object from a file")
	fmt.Println("   cat-file     Provide content or type and size information for repository objects")
	fmt.Println("   add          Add file contents to the index")
	fmt.Println("   rm           Remove files from the working tree and from the index")
	fmt.Println("   write-tree   Create a tree object from the current index")
	fmt.Println("   commit       Record changes to the repository")
	fmt.Println("   status       Show the working tree status")
	fmt.Println("   log          Show commit logs")
	fmt.Println("   branch       List, create, or delete branches")
	fmt.Println("   checkout     Switch branches or restore working tree files")
	fmt.Println("   diff         Show changes between commits, commit and working tree, etc")
	fmt.Println("   tag          Create or list tags")
}

func handleInit(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	r, err := repo.Init(dir)
	if err != nil {
		return err
	}
	fmt.Printf("Initialized empty Git repository in %s\n", r.GitDir)
	return nil
}

func handleHashObject(args []string) error {
	fs := flag.NewFlagSet("hash-object", flag.ExitOnError)
	writeFlag := fs.Bool("w", false, "write object to database")
	objType := fs.String("t", "blob", "object type")
	_ = fs.Parse(args)

	files := fs.Args()
	if len(files) == 0 {
		return fmt.Errorf("no file specified")
	}

	var r *repo.Repository
	if *writeFlag {
		var err error
		r, err = repo.FindRepo(".")
		if err != nil {
			return err
		}
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		if *writeFlag {
			hash, err := object.WriteObject(r.GitDir, *objType, data)
			if err != nil {
				return err
			}
			fmt.Println(hash)
		} else {
			hash, _ := object.HashObject(*objType, data)
			fmt.Println(hash)
		}
	}
	return nil
}

func handleCatFile(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: minigit cat-file (-p|-t|-s) <object>")
	}

	flag := args[0]
	hashPrefix := args[1]

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	objType, data, _, err := object.ReadObject(r.GitDir, hashPrefix)
	if err != nil {
		return err
	}

	switch flag {
	case "-p":
		if objType == object.TypeTree {
			tree, err := object.ParseTree(data)
			if err != nil {
				return err
			}
			for _, entry := range tree.Entries {
				fmt.Printf("%s %s %s\t%s\n", entry.Mode, entryTypeFromMode(entry.Mode), entry.Hash, entry.Name)
			}
		} else {
			os.Stdout.Write(data)
		}
	case "-t":
		fmt.Println(objType)
	case "-s":
		fmt.Println(len(data))
	default:
		return fmt.Errorf("unknown cat-file option: %s", flag)
	}

	return nil
}

func entryTypeFromMode(mode string) string {
	if mode == "40000" {
		return "tree"
	}
	return "blob"
}

func handleAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("nothing specified, nothing added")
	}

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	return r.Add(args)
}

func handleRm(args []string) error {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	cached := fs.Bool("cached", false, "unstage only")
	_ = fs.Parse(args)

	files := fs.Args()
	if len(files) == 0 {
		return fmt.Errorf("no files specified to rm")
	}

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	return r.Remove(files, *cached)
}

func handleWriteTree(args []string) error {
	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	idx, err := r.LoadIndex()
	if err != nil {
		return err
	}

	treeHash, err := idx.BuildTree(r.GitDir)
	if err != nil {
		return err
	}

	fmt.Println(treeHash)
	return nil
}

func handleCommit(args []string) error {
	fs := flag.NewFlagSet("commit", flag.ExitOnError)
	msg := fs.String("m", "", "commit message")
	author := fs.String("author", "", "author info")
	_ = fs.Parse(args)

	if *msg == "" {
		return fmt.Errorf("commit message cannot be empty (use -m <msg>)")
	}

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	hash, err := r.Commit(*msg, *author)
	if err != nil {
		return err
	}

	branch, _, _, _ := r.ResolveHEAD()
	firstLine := strings.SplitN(*msg, "\n", 2)[0]
	fmt.Printf("[%s %s] %s\n", branch, hash[:7], firstLine)
	return nil
}

func handleStatus(args []string) error {
	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	st, err := r.Status()
	if err != nil {
		return err
	}

	if st.IsDetached {
		fmt.Printf("HEAD detached at %s\n", st.HEADHash[:7])
	} else {
		fmt.Printf("On branch %s\n", st.Branch)
	}

	if st.Clean {
		fmt.Println("nothing to commit, working tree clean")
		return nil
	}

	if len(st.Staged) > 0 {
		fmt.Println("Changes to be committed:")
		fmt.Println("  (use \"minigit rm --cached <file>...\" to unstage)")
		for _, s := range st.Staged {
			fmt.Printf("\t%s:   %s\n", colorGreen(string(s.Status)), s.Path)
		}
		fmt.Println()
	}

	if len(st.Unstaged) > 0 {
		fmt.Println("Changes not staged for commit:")
		fmt.Println("  (use \"minigit add <file>...\" to update what will be committed)")
		for _, s := range st.Unstaged {
			fmt.Printf("\t%s:   %s\n", colorRed(string(s.Status)), s.Path)
		}
		fmt.Println()
	}

	if len(st.Untracked) > 0 {
		fmt.Println("Untracked files:")
		fmt.Println("  (use \"minigit add <file>...\" to include in what will be committed)")
		for _, u := range st.Untracked {
			fmt.Printf("\t%s\n", colorRed(u))
		}
		fmt.Println()
	}

	return nil
}

func handleLog(args []string) error {
	fs := flag.NewFlagSet("log", flag.ExitOnError)
	oneline := fs.Bool("oneline", false, "single line format")
	count := fs.Int("n", 0, "max commits")
	_ = fs.Parse(args)

	startRev := ""
	if len(fs.Args()) > 0 {
		startRev = fs.Args()[0]
	}

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	logs, err := r.Log(startRev, *count)
	if err != nil {
		return err
	}

	for i, entry := range logs {
		if *oneline {
			firstLine := strings.SplitN(entry.Commit.Message, "\n", 2)[0]
			fmt.Printf("%s %s\n", entry.Hash[:7], firstLine)
		} else {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("%s\n", colorYellow("commit "+entry.Hash))
			fmt.Printf("Author: %s\n", entry.Commit.Author)
			fmt.Printf("Date:   %s\n\n", entry.Commit.AuthorTime.Format("Mon Jan 2 15:04:05 2006 -0700"))
			lines := strings.Split(strings.TrimSpace(entry.Commit.Message), "\n")
			for _, line := range lines {
				fmt.Printf("    %s\n", line)
			}
		}
	}

	return nil
}

func handleBranch(args []string) error {
	fs := flag.NewFlagSet("branch", flag.ExitOnError)
	del := fs.String("d", "", "delete branch")
	_ = fs.Parse(args)

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	if *del != "" {
		if err := r.DeleteBranch(*del); err != nil {
			return err
		}
		fmt.Printf("Deleted branch %s\n", *del)
		return nil
	}

	rest := fs.Args()
	if len(rest) == 0 {
		branches, curBranch, err := r.ListBranches()
		if err != nil {
			return err
		}
		for _, b := range branches {
			if b == curBranch {
				fmt.Printf("* %s\n", colorGreen(b))
			} else {
				fmt.Printf("  %s\n", b)
			}
		}
		return nil
	}

	newBranch := rest[0]
	_, _, headHash, err := r.ResolveHEAD()
	if err != nil {
		return err
	}
	if headHash == "" {
		return fmt.Errorf("cannot create branch '%s' without commits", newBranch)
	}

	if err := r.CreateBranch(newBranch, headHash); err != nil {
		return err
	}
	return nil
}

func handleCheckout(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: minigit checkout <branch-or-commit>")
	}

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	target := args[0]
	if err := r.Checkout(target); err != nil {
		return err
	}

	branch, isSymbolic, hash, _ := r.ResolveHEAD()
	if isSymbolic {
		fmt.Printf("Switched to branch '%s'\n", branch)
	} else {
		fmt.Printf("Note: switching to '%s'. You are in 'detached HEAD' state.\n", hash[:7])
	}
	return nil
}

func handleDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	staged := fs.Bool("staged", false, "view staged changes")
	cached := fs.Bool("cached", false, "view staged changes")
	_ = fs.Parse(args)

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	d, err := r.Diff(*staged || *cached)
	if err != nil {
		return err
	}

	if d != "" {
		fmt.Print(d)
	}
	return nil
}

func handleTag(args []string) error {
	fs := flag.NewFlagSet("tag", flag.ExitOnError)
	del := fs.String("d", "", "delete tag")
	_ = fs.Parse(args)

	r, err := repo.FindRepo(".")
	if err != nil {
		return err
	}

	tagsDir := os.DirFS(r.GitDir)
	_ = tagsDir

	if *del != "" {
		tagPath := r.GitDir + "/refs/tags/" + *del
		if _, err := os.Stat(tagPath); os.IsNotExist(err) {
			return fmt.Errorf("tag '%s' not found", *del)
		}
		return os.Remove(tagPath)
	}

	rest := fs.Args()
	if len(rest) == 0 {
		entries, err := os.ReadDir(r.GitDir + "/refs/tags")
		if err == nil {
			for _, e := range entries {
				fmt.Println(e.Name())
			}
		}
		return nil
	}

	tagName := rest[0]
	_, _, headHash, err := r.ResolveHEAD()
	if err != nil {
		return err
	}
	if headHash == "" {
		return fmt.Errorf("cannot create tag '%s' without commits", tagName)
	}

	if len(rest) > 1 {
		resolved, err := object.ResolveObjectHash(r.GitDir, rest[1])
		if err != nil {
			return err
		}
		headHash = resolved
	}

	tagPath := r.GitDir + "/refs/tags/" + tagName
	if _, err := os.Stat(tagPath); err == nil {
		return fmt.Errorf("tag '%s' already exists", tagName)
	}
	return os.WriteFile(tagPath, []byte(headHash+"\n"), 0644)
}

func parseOffset(s string) int {
	if len(s) != 5 {
		return 0
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	hours, _ := strconv.Atoi(s[1:3])
	mins, _ := strconv.Atoi(s[3:5])
	return sign * (hours*3600 + mins*60)
}

func colorGreen(s string) string {
	if isTTY() {
		return "\x1b[32m" + s + "\x1b[0m"
	}
	return s
}

func colorRed(s string) string {
	if isTTY() {
		return "\x1b[31m" + s + "\x1b[0m"
	}
	return s
}

func colorYellow(s string) string {
	if isTTY() {
		return "\x1b[33m" + s + "\x1b[0m"
	}
	return s
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

