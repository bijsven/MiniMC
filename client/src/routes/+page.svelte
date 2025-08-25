<script lang="ts">
    import { Xterm, XtermAddon } from "@battlefieldduck/xterm-svelte";
    import type {
        ITerminalOptions,
        Terminal,
    } from "@battlefieldduck/xterm-svelte";
    import FileManager from "./FileManager.svelte";

    let terminal: Terminal = $state() as Terminal;
    let eventSource: EventSource;
    let showFileManager = $state(false);

    let input = $state("");
    const placeholder =
        "enter a command... (start with / to open minimc commands)";

    let options: ITerminalOptions = {
        fontFamily: "Consolas",
        theme: { background: "#242424", foreground: "#ffffff" },
        disableStdin: true,
        altClickMovesCursor: false,
        customGlyphs: false,
    };

    function color(text: string, code: number) {
        return `\x1b[${code}m${text}\x1b[0m`;
    }

    function parseLogLine(line: string) {
        if (line.startsWith("[i]")) {
            return color(line, 32);
        } else if (line.startsWith("[e]")) {
            return color(line, 31);
        } else if (line.startsWith("[g]")) {
            if (/ERROR/i.test(line)) return color(line, 31);
            if (/WARN(ING)?/i.test(line)) return color(line, 33);
            if (/INFO/i.test(line)) return color(line, 36);
            return color(line, 37);
        }

        // extra keywords
        if (/Done|Ready/i.test(line)) {
            return color(line, 32); // groen
        }

        return line; // default
    }

    async function connectLogs() {
        eventSource = new EventSource("/api/logs");

        eventSource.onmessage = (event) => {
            if (terminal) {
                terminal.writeln(parseLogLine(event.data));
            }
        };

        eventSource.onerror = (err) => {
            console.error("SSE error", err);
            eventSource.close();
            setTimeout(connectLogs, 3000); // retry na 3s
        };
    }

    async function handleCommand(command: string) {
        if (!command.trim()) return;

        if (command.trim() === "/files") {
            showFileManager = true;
            return;
        }

        terminal.writeln(`${color("$", 33)} ${command}`);

        try {
            const res = await fetch("/api/command", {
                method: "POST",
                headers: {
                    "Content-Type": "application/x-www-form-urlencoded",
                },
                body: new URLSearchParams({ command }),
            });
            if (!res.ok) {
                terminal.writeln(
                    `${color(`Network Error: ${res.status}`, 31)}`,
                );
            }
        } catch (err) {
            terminal.writeln(`${color(`Error: ${err}`, 31)}`);
        }
    }

    function onInputKey(event: KeyboardEvent) {
        if (event.key === "Enter") {
            handleCommand(input);
            input = "";
        }
    }

    async function onLoad() {
        const fitAddon = new (await XtermAddon.FitAddon()).FitAddon();
        terminal.loadAddon(fitAddon);
        fitAddon.fit();

        terminal.writeln(`${color("app@minimc~", 32)} Welcome to MiniMC!`);
        terminal.writeln(`${color("app@minimc~", 32)} connecting...`);

        connectLogs();
    }
</script>

<div class="relative w-full h-full flex flex-col overflow-clip p-5">
    <div
        class="flex-1 overflow-hidden"
        onclick={() => {
            terminal.write("\x1b[?25l"); // cursor verbergen
        }}
        role="presentation"
    >
        <Xterm bind:terminal class="h-full w-full" {options} {onLoad} />
    </div>
    <div class="h-8 text-white flex items-center px-2 font-mono text-base">
        <span>$ </span>
        <input
            bind:value={input}
            onkeydown={onInputKey}
            {placeholder}
            autocomplete="off"
            autocorrect="off"
            spellcheck="false"
            class="bg-transparent border-none outline-0 border-0 ring-0 text-white flex-1 outline-none"
        />
    </div>
</div>

<FileManager
    show={showFileManager}
    {terminal}
    onClose={() => (showFileManager = false)}
/>
