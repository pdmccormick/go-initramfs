package main

import (
	"bytes"
	"context"
	"debug/elf"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"go.pdmccormick.com/initramfs"
)

func main() {
	// This program leads a double life as a userland /init
	if os.Getpid() == 1 {
		Pid1Init()
		return
	}

	var (
		pid1Flag    = flag.Bool("init", false, "run the init portion of this program")
		initrdFlag  = flag.String("initrd", "linuxboot.initramfs", "`path` to write initramfs to")
		kernelFlag  = flag.String("kernel", fmt.Sprintf(DefaultKernelSearchPathFmt, unameKernelRelease), "`path` to kernel image")
		e1000koFlag = flag.String("e1000ko", "", "`path` to e1000 .ko module")
		noSudoFlag  = flag.Bool("nosudo", false, "do not invoke qemu via sudo")
		cmdlineFlag = flag.String("cmdline", "console=ttyS0,115200", "kernel cmdline")
		appendFlag  = flag.String("append", "", "append more kernel cmdline")
		stdinFlag   = flag.Bool("stdin", false, "attach stdin to QEMU")
		noRunFlag   = flag.Bool("norun", false, "generate initrd and show QEMU arguments, but do not run them")
		childIpFlag = flag.String("childip", "10.80.68.77/30", "IP address of child VM (host will be assigned next address)")

		tapFlag = flag.String("tap", "qemulinuxboot", "TAP interface name")
	)

	flag.Parse()

	if *pid1Flag {
		Pid1Init()
		return
	}

	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	selfExe = exe

	childIp, err := netip.ParsePrefix(*childIpFlag)
	if err != nil {
		log.Fatalf("Bad `-childip` prefix: %s", err)
	}

	hostIp := netip.PrefixFrom(childIp.Addr().Next(), childIp.Bits())

	cmdline := fmt.Sprintf("%s %s hostip=%s ip=%s", *cmdlineFlag, *appendFlag, hostIp, childIp)

	if *e1000koFlag == "" {
		*e1000koFlag = fmt.Sprintf(DefaultE1000SearchPathFmt, unameKernelRelease)
	}

	if err := generateInitrd(*initrdFlag, *e1000koFlag); err != nil {
		log.Fatalf("generateInitrd: %s", err)
	}

	qemuArgs := []string{
		"sudo",
		"qemu-system-x86_64",
		"-kernel", *kernelFlag,
		"-initrd", *initrdFlag,
		"-serial", "mon:stdio",
		"-nographic",
		"-netdev", "tap,id=net0,ifname=" + *tapFlag + ",script=no",
		"-device", "e1000,netdev=net0,mac=12:34:56:78:9a:bc",
		"-append", cmdline,
	}

	if *noSudoFlag {
		qemuArgs = qemuArgs[1:]
	}

	qemuArgs = append(qemuArgs, flag.Args()...)

	quoteArgs := shellescape.QuoteCommand(qemuArgs)

	staticCheck := checkExeStatic()

	if *noRunFlag {
		if staticCheck != nil {
			log.Printf("WARNING: this initrd is broken: %s\n\n", staticCheck)
		}

		fmt.Printf("%s\n", quoteArgs)

		return
	}

	if staticCheck != nil {
		log.Fatal(staticCheck)
	}

	log.Printf("Run: %s", quoteArgs)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd := exec.Command(qemuArgs[0], qemuArgs[1:]...)
	// cmd.Stdin = os.Stdin
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if *stdinFlag {
		pr, pw, err := os.Pipe()
		if err != nil {
			log.Fatalf("Pipe: %s", err)
		}

		cmd.Stdin = pr

		defer pw.Close()
		defer pr.Close()

		go func() {
			defer pw.Close()
			io.Copy(pw, os.Stdin)
		}()
	}

	var runErrc = make(chan error, 1)
	go func() {
		runErrc <- cmd.Run()
	}()

	time.Sleep(1 * time.Second)

	log.Printf("HOST: Run: %v", qemuArgs)

	setupHostNetwork(*tapFlag, hostIp)

	var (
		donec = ctx.Done()
		killc <-chan time.Time
	)

	for {
		select {
		case <-donec:
			donec = nil
			log.Printf("HOST: Stopping...")
			stop()
			cmd.Process.Signal(os.Interrupt)
			// return

			killc = time.NewTimer(3 * time.Second).C

		case <-killc:
			killc = nil
			log.Printf("HOST: Killing...")
			cmd.Process.Kill()

		case err := <-runErrc:
			if err != nil {
				log.Printf("Run: %s", err)
			}
			return
		}
	}
}

var (
	selfExe            string
	uname              unix.Utsname
	unameKernelRelease string // The value of `uname -r`
)

const (
	DefaultE1000SearchPathFmt  = `/usr/lib/modules/%s/kernel/drivers/net/ethernet/intel/e1000/e1000.ko`
	DefaultKernelSearchPathFmt = `/boot/vmlinuz-%s`
)

func init() {
	if err := unix.Uname(&uname); err == nil {
		unameKernelRelease = string(bytes.TrimRight(uname.Release[:], "\x00"))
	}
}

// Ensure that this executable is statically linked, otherwise it will fail to exec as /init
func checkExeStatic() error {
	f, err := os.Open(selfExe)
	if err != nil {
		return err
	}
	defer f.Close()

	ef, err := elf.NewFile(f)
	if err != nil {
		return fmt.Errorf("elf.Newfile %s: %w", selfExe, err)
	}

	ef.Close()

	syms, err := ef.DynamicSymbols()
	if err != nil {
		return nil
	}

	if len(syms) == 0 {
		return nil
	}

	return fmt.Errorf("%s is not a statically compiled binary, try building with `CGO_ENABLED=0 go build .`", selfExe)
}

// Create an initramfs archive containing this executable as /init
func generateInitrd(name, e1000ko string) error {
	exeData, err := os.ReadFile(selfExe)
	if err != nil {
		return fmt.Errorf("ReadFile %s: %w", selfExe, err)
	}

	f, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("Create %s: %w", name, err)
	}

	defer f.Close()

	iw := initramfs.NewWriter(f)

	defer iw.Close()

	if err := iw.StartCompression(initramfs.GzipWriter); err != nil {
		panic(err)
	}

	// Add /init
	var hdr = initramfs.Header{
		Mode:     initramfs.Mode_File | 0o700,
		DataSize: uint32(len(exeData)),
		Filename: "/init",
	}

	if err := iw.WriteHeader(&hdr); err != nil {
		return fmt.Errorf("WriteHeader: %w", err)
	}

	if _, err := iw.Write(exeData); err != nil {
		return fmt.Errorf("Write: %w", err)
	}

	// Add /e1000.ko (if exists)
	koData, err := os.ReadFile(e1000ko)
	if err != nil {
		log.Printf("WARNING: %s does not exist, pass in `-e1000ko` flag")
	} else {
		var hdr = initramfs.Header{
			Mode:     initramfs.Mode_File | 0o700,
			DataSize: uint32(len(koData)),
			Filename: "/e1000.ko",
		}

		if err := iw.WriteHeader(&hdr); err != nil {
			return fmt.Errorf("WriteHeader: %w", err)
		}

		if _, err := iw.Write(koData); err != nil {
			return fmt.Errorf("Write: %w", err)
		}
	}

	if err := iw.WriteTrailer(); err != nil {
		return err
	}

	return nil
}

func setupHostNetwork(ifname string, ip netip.Prefix) {
	args := []string{
		"sudo",
		"ip", "link", "set", "up", "dev", ifname,
	}

	log.Printf("HOST: Bringing up interface %s with: %s", ifname, shellescape.QuoteCommand(args))

	if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
		log.Printf("Error: %s", err)
		return
	}

	args = []string{
		"sudo",
		"ip", "addr", "add", ip.String(), "dev", ifname,
	}

	log.Printf("HOST: Adding interface address with: %s", shellescape.QuoteCommand(args))

	if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
		log.Printf("Error: %s", err)
		return
	}
}

////////////////////////////////////////////////////////////////////

// Everything below is run in the context of a PID 1 /init process

func Pid1Init() {
	time.Sleep(1 * time.Second)
	fmt.Printf("\n\n")
	log.Printf("Welcome to the qemulinuxboot VM!\n\n")

	log.Printf("Args:")
	for _, arg := range os.Args {
		log.Printf("\t%s", arg)
	}

	log.Printf("Environ:")
	for _, env := range os.Environ() {
		log.Printf("\t%s", env)
	}

	fmt.Printf("\n")

	log.Printf("Mounting core filesystems")
	mountCore()

	log.Printf("Loading E1000 driver")
	loadE1000Module()

	if ip, ok := setupVMNetwork(); ok {
		setupHttp()

		log.Printf("Network ready, visit http://%s", ip)

		if err := http.ListenAndServe(":80", nil); err != nil {
			log.Printf("http: %s", err)
		}
	}

	for range time.NewTicker(3 * time.Second).C {
		log.Printf("Still alive")
	}
}

// Mount all core filesystems
func mountCore() {
	// nosuid,relatime,nodev,noexec
	mustMount("udev", "/dev", "devtmpfs", "rw")
	mustMount("proc", "/proc", "proc", "rw")
	mustMount("sysfs", "/sys", "sysfs", "rw")
	// mustMount("efivarfs", "/sys/firmware/efi/efivars", "efivarfs", "rw")
}

func mount(source, target, fstype, options string, flags int) error {
	log.Printf("mount -t %s %s %s", fstype, source, target)

	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("mkdir `%s`: %s", target, err)
	}

	return unix.Mount(source, target, fstype, uintptr(flags), options)
}

func mustMount(source, target, fstype, options string) {
	if err := mount(source, target, fstype, options, 0); err != nil {
		log.Fatalf("failed to mount `%s` (%s) => `%s`: %s", source, fstype, target, err)
	}
}

func loadE1000Module() {
	data, err := os.ReadFile("/e1000.ko")
	if err != nil {
		log.Printf("WARNING: unable to read /e1000.ko, not loading network driver")
		return
	}

	if err := unix.InitModule(data, ""); err != nil {
		log.Printf("ERROR: unable to load e1000 module: %s", err)
	}
}

func setupVMNetwork() (ipStr string, ok bool) {
	link, err := netlink.LinkByName("eth0")
	if err != nil {
		log.Printf("Unable to find eth0 interface")

		names, err := filepath.Glob("/sys/class/net/*")
		if err == nil {
			log.Printf("Names under /sys/class/net:")
			for _, name := range names {
				log.Printf("\t%s", filepath.Base(name))
			}
		}

		return
	}

	var linkName = link.Attrs().Name

	log.Printf("Bringing up %s", linkName)
	if err := netlink.LinkSetUp(link); err != nil {
		log.Printf("Error: %s", err)
		return
	}

	var envIp = os.Getenv("ip")
	ip, err := netip.ParsePrefix(envIp)
	if err != nil {
		log.Printf("Error parsing `ip=` environ `%s`: %s", envIp, err)
		return
	}

	ip0, ipNet, err := net.ParseCIDR(envIp)
	ipNet.IP = ip0
	if err != nil {
		panic(err)
	}

	var addr = netlink.Addr{
		IPNet: ipNet,
	}

	if err := netlink.AddrAdd(link, &addr); err != nil {
		log.Printf("Error assigning %s to %s: %s", ip, linkName, err)
		return
	}

	log.Printf("Interface %s up with address %s", linkName, addr)

	return ip0.String(), true
}

func setupHttp() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "/debug/pprof\n")
		fmt.Fprintf(w, `<!doctype html>
<meta name="viewport" content="width=device-width">
<pre>
<a href='/debug/pprof'>debug/pprof</a>
<a href='/rootfs'>rootfs</a>
</pre>
`)
	})

	var srv = Server{
		path: "/rootfs",
		root: os.DirFS("/"),
	}

	http.Handle("/rootfs/", &srv)
}

type Server struct {
	path string
	root fs.FS
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var path = r.URL.Path
	path = strings.TrimPrefix(path, srv.path)
	path = strings.TrimLeft(path, "/")

	info, err := fs.Stat(srv.root, filepath.Join(".", path))
	if err != nil {
		http.Error(w, fmt.Sprintf("Stat: %s", err), http.StatusInternalServerError)
		return
	}

	if info.IsDir() {
		path = strings.TrimSuffix(path, "/")

		if path == "" {
			path = "."
		}

		var base = filepath.Join(srv.path, path)

		ents, err := fs.ReadDir(srv.root, path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, `<!doctype html>
<meta name="viewport" content="width=device-width">
<pre>
`)

		for _, ent := range ents {
			var (
				name    = ent.Name()
				relpath = filepath.Join(path, name)
				disp    = name
			)

			if fi, err := ent.Info(); err == nil {
				if (fi.Mode() & fs.ModeSymlink) == fs.ModeSymlink {
					target, err := os.Readlink("/" + relpath)
					if err == nil {
						disp += " -> " + string(target)
					}
				}
			}

			fmt.Fprintf(w, "<a href='%s'>%s</a>\n", filepath.Join(base, name), disp)
		}

		fmt.Fprintf(w, `</pre>`)
		return
	}

	// Dump the file.
	f, err := srv.root.Open(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Open: %s", err), http.StatusInternalServerError)
		return
	}

	defer f.Close()

	io.Copy(w, f)
}
