package explicitness

// ambientInputs are standard library functions and variables that give data
// from the environment: the process, the clock, random sources, the terminal,
// the file system and the network.
var ambientInputs = setOf(
	"os.Args", "os.Stdin", "os.Getenv", "os.LookupEnv", "os.Environ",
	"os.ReadFile", "os.ReadDir", "os.Open", "os.OpenFile", "os.Stat", "os.Lstat",
	"os.Getwd", "os.Hostname", "os.Getpid", "os.Getppid", "os.Getuid",
	"os.Geteuid", "os.Getgid", "os.Getegid", "os.Executable", "os.UserHomeDir",
	"os.UserConfigDir", "os.UserCacheDir", "os.TempDir",
	"time.Now", "time.Since", "time.Until", "time.After", "time.Tick",
	"math/rand.Int", "math/rand.Intn", "math/rand.Int31", "math/rand.Int31n",
	"math/rand.Int63", "math/rand.Int63n", "math/rand.Uint32", "math/rand.Uint64",
	"math/rand.Float32", "math/rand.Float64", "math/rand.Perm", "math/rand.Shuffle",
	"math/rand.NormFloat64", "math/rand.ExpFloat64",
	"crypto/rand.Read", "crypto/rand.Int", "crypto/rand.Prime", "crypto/rand.Text",
	"crypto/rand.Reader",
	"fmt.Scan", "fmt.Scanf", "fmt.Scanln",
	"flag.Args", "flag.Arg", "flag.NArg", "flag.Parse",
	"net/http.Get", "net/http.Head", "net/http.Post", "net/http.PostForm",
	"path/filepath.Walk", "path/filepath.WalkDir", "path/filepath.Glob",
	"path/filepath.Abs",
	"io/ioutil.ReadFile", "io/ioutil.ReadDir",
	"os/exec.Command", "os/exec.CommandContext", "os/exec.LookPath",
	"net.Dial", "net.DialTimeout", "net.Listen", "net.LookupHost", "net.LookupIP",
)

// ambientOutputs are standard library functions and variables that change the
// environment: the terminal, the logs, the process, the file system and the
// network.
var ambientOutputs = setOf(
	"fmt.Print", "fmt.Printf", "fmt.Println",
	"log.Print", "log.Printf", "log.Println", "log.Fatal", "log.Fatalf",
	"log.Fatalln", "log.Panic", "log.Panicf", "log.Panicln", "log.SetOutput",
	"log.SetFlags", "log.SetPrefix", "log.Output",
	"log/slog.Info", "log/slog.Warn", "log/slog.Error", "log/slog.Debug",
	"log/slog.Log", "log/slog.LogAttrs", "log/slog.InfoContext",
	"log/slog.WarnContext", "log/slog.ErrorContext", "log/slog.DebugContext",
	"log/slog.SetDefault",
	"os.Stdout", "os.Stderr", "os.Exit", "os.WriteFile", "os.Create",
	"os.OpenFile", "os.Remove", "os.RemoveAll", "os.Mkdir", "os.MkdirAll",
	"os.MkdirTemp", "os.CreateTemp", "os.Rename", "os.Chdir", "os.Chmod",
	"os.Chown", "os.Chtimes", "os.Setenv", "os.Unsetenv", "os.Clearenv",
	"os.Symlink", "os.Link", "os.Truncate",
	"net/http.Get", "net/http.Head", "net/http.Post", "net/http.PostForm",
	"net/http.ListenAndServe", "net/http.ListenAndServeTLS", "net/http.Handle",
	"net/http.HandleFunc",
	"io/ioutil.WriteFile",
	"os/exec.Command", "os/exec.CommandContext",
	"net.Dial", "net.DialTimeout", "net.Listen",
)

func setOf(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// streamWriters maps each standard library function that writes to a stream
// to the index of the stream argument.
var streamWriters = map[string]int{
	"fmt.Fprint": 0, "fmt.Fprintf": 0, "fmt.Fprintln": 0,
	"io.WriteString": 0, "io.Copy": 0, "io.CopyN": 0, "io.CopyBuffer": 0,
}

// streamReaders maps each standard library function that reads from a stream
// to the index of the stream argument.
var streamReaders = map[string]int{
	"fmt.Fscan": 0, "fmt.Fscanf": 0, "fmt.Fscanln": 0,
	"io.ReadAll": 0, "io.ReadFull": 0, "io.ReadAtLeast": 0,
	"io.Copy": 1, "io.CopyN": 1, "io.CopyBuffer": 1,
}

// builtinWrites are the builtins that change the data that their first
// argument refers to. The value is true if the builtin does not also read that
// data.
var builtinWrites = map[string]bool{"delete": true, "clear": true, "copy": false}

// inPlaceSorts are standard library functions that change the order of the
// elements of their first argument.
var inPlaceSorts = setOf(
	"sort.Slice", "sort.SliceStable", "sort.Sort", "sort.Stable", "sort.Strings",
	"sort.Ints", "sort.Float64s",
	"slices.Sort", "slices.SortFunc", "slices.SortStableFunc", "slices.Reverse",
)
