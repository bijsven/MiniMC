<script lang="ts">
    export let show = false;
    export let onClose: () => void;
    export let terminal: any;

    let currentPath = "";
    let files: FileInfo[] = [];
    let selectedFile: FileInfo | null = null;
    let fileContent = "";
    let isEditing = false;
    let newFileName = "";
    let newFolderName = "";
    let showCreateMenu = false;
    let loading = false;
    let error = "";
    let isDragOver = false;

    interface FileInfo {
        name: string;
        path: string;
        is_dir: boolean;
        size: number;
        mod_time: string;
        extension?: string;
    }

    function color(text: string, code: number) {
        return `\x1b[${code}m${text}\x1b[0m`;
    }

    function log(message: string) {
        terminal?.writeln(message);
    }

    async function loadFiles(path: string = "") {
        loading = true;
        error = "";
        try {
            const url = path
                ? `/api/files?path=${encodeURIComponent(path)}`
                : "/api/files";
            const res = await fetch(url);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const rawFiles = await res.json();

            files = rawFiles.sort((a: FileInfo, b: FileInfo) => {
                if (a.is_dir && !b.is_dir) return -1;
                if (!a.is_dir && b.is_dir) return 1;
                return a.name.localeCompare(b.name);
            });

            currentPath = path;
        } catch (err) {
            error = err instanceof Error ? err.message : "Unknown error";
            console.error(err);
        }
        loading = false;
    }

    let notifications: string[] = [];

    function addNotification(
        message: string,
        type: "error" | "warning" | "info" = "info",
    ) {
        const prefix =
            type === "error"
                ? "[ERROR]"
                : type === "warning"
                  ? "[WARN]"
                  : "[INFO]";
        const notification = `${prefix} ${message}`;
        notifications = [...notifications, notification];
        log(
            color(
                notification,
                type === "error" ? 31 : type === "warning" ? 33 : 32,
            ),
        );

        setTimeout(() => {
            notifications = notifications.filter((n) => n !== notification);
        }, 5000);
    }

    function isTextFile(filename: string): boolean {
        const textExtensions = [
            ".txt",
            ".md",
            ".yml",
            ".yaml",
            ".json",
            ".js",
            ".ts",
            ".html",
            ".css",
            ".xml",
            ".csv",
            ".log",
            ".cfg",
            ".conf",
            ".ini",
        ];
        const ext = "." + (filename.split(".").pop()?.toLowerCase() || "");
        return textExtensions.includes(ext);
    }

    function isArchive(filename: string): boolean {
        const archiveExtensions = [".tar.gz", ".tgz"];
        const lowerName = filename.toLowerCase();
        return archiveExtensions.some((ext) => lowerName.endsWith(ext));
    }

    async function extractArchive(file: FileInfo) {
        if (
            !confirm(
                `Extract ${file.name}? This will extract all files to the current directory.`,
            )
        )
            return;

        try {
            const res = await fetch("/api/files/extract", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    path: file.path,
                    destination: currentPath,
                }),
            });

            if (!res.ok) {
                const errorData = await res.json();
                throw new Error(errorData.message || `HTTP ${res.status}`);
            }

            const result = await res.json();
            addNotification(
                `Extracted ${result.count} files from '${file.name}'`,
                "info",
            );
            log(
                color(`Extracted ${result.count} files from: ${file.name}`, 32),
            );
            await loadFiles(currentPath);
        } catch (err) {
            addNotification(
                `Failed to extract '${file.name}': ${err}`,
                "error",
            );
            log(color(`Failed to extract archive: ${err}`, 31));
        }
    }

    async function select(file: FileInfo) {
        selectedFile = file;
        fileContent = "";
        isEditing = false;
        if (!file.is_dir) {
            if (file.size > 5 * 1024 * 1024) {
                addNotification(
                    `File '${file.name}' is ${formatFileSize(file.size)} - too large to preview (max: 5MB)`,
                    "warning",
                );
                fileContent = `[File too large to preview - ${formatFileSize(file.size)}]\n\nUse external editor to view this file.`;
                return;
            }

            if (!isTextFile(file.name)) {
                addNotification(
                    `File '${file.name}' is not a text file - cannot preview binary content`,
                    "warning",
                );
                fileContent = `[Binary file - cannot preview]\n\nFile type: ${file.name.split(".").pop()?.toUpperCase() || "Unknown"}\nSize: ${formatFileSize(file.size)}`;
                return;
            }

            try {
                const res = await fetch(
                    `/api/files/content?path=${encodeURIComponent(file.path)}`,
                );
                if (res.ok) {
                    fileContent = (await res.json()).content;
                    addNotification(
                        `Loaded '${file.name}' (${formatFileSize(file.size)})`,
                        "info",
                    );
                }
            } catch (err) {
                addNotification(
                    `Failed to load '${file.name}': ${err}`,
                    "error",
                );
                console.error(err);
            }
        }
    }

    async function openFolder(folder: FileInfo) {
        if (folder.is_dir) await loadFiles(folder.path);
        selectedFile = null;
        fileContent = "";
        isEditing = false;
    }

    async function goBack() {
        const parent = currentPath.split("/").slice(0, -1).join("/");
        await loadFiles(parent);
        selectedFile = null;
        fileContent = "";
        isEditing = false;
    }

    async function saveFile() {
        if (!selectedFile) return;
        try {
            const res = await fetch("/api/files/content", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    path: selectedFile.path,
                    content: fileContent,
                }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            isEditing = false;
            addNotification(
                `Saved '${selectedFile.name}' successfully`,
                "info",
            );
            log(color(`File saved: ${selectedFile.name}`, 32));
            await loadFiles(currentPath);
        } catch (err) {
            addNotification(
                `Failed to save '${selectedFile.name}': ${err}`,
                "error",
            );
            log(color(`Failed to save file: ${err}`, 31));
        }
    }

    async function deleteFile(file: FileInfo) {
        if (!confirm(`Delete ${file.name}?`)) return;
        try {
            const res = await fetch(
                `/api/files?path=${encodeURIComponent(file.path)}`,
                { method: "DELETE" },
            );
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            addNotification(`Deleted '${file.name}' successfully`, "info");
            log(color(`Deleted: ${file.name}`, 32));
            await loadFiles(currentPath);
            if (selectedFile?.path === file.path) selectedFile = null;
        } catch (err) {
            addNotification(`Failed to delete '${file.name}': ${err}`, "error");
            log(color(`Failed to delete: ${err}`, 31));
        }
    }

    async function createFile() {
        if (!newFileName.trim()) return;
        try {
            const path = currentPath
                ? `${currentPath}/${newFileName}`
                : newFileName;
            const res = await fetch("/api/files/content", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ path, content: "" }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            log(color(`Created file: ${newFileName}`, 32));
            newFileName = "";
            showCreateMenu = false;
            await loadFiles(currentPath);
        } catch (err) {
            log(color(`Failed to create file: ${err}`, 31));
        }
    }

    async function createFolder() {
        if (!newFolderName.trim()) return;
        try {
            const path = currentPath
                ? `${currentPath}/${newFolderName}`
                : newFolderName;
            const res = await fetch("/api/files/mkdir", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ path }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            log(color(`Created folder: ${newFolderName}`, 32));
            newFolderName = "";
            showCreateMenu = false;
            await loadFiles(currentPath);
        } catch (err) {
            log(color(`Failed to create folder: ${err}`, 31));
        }
    }

    function formatFileSize(bytes: number) {
        const units = ["B", "KB", "MB", "GB"];
        let i = 0;
        while (bytes >= 1024 && i < units.length - 1) {
            bytes /= 1024;
            i++;
        }
        return `${bytes.toFixed(1)}${units[i]}`;
    }

    function handleDragOver(e: DragEvent) {
        e.preventDefault();
        isDragOver = true;
    }

    function handleDragLeave(e: DragEvent) {
        e.preventDefault();
        if (
            !e.relatedTarget ||
            !(e.currentTarget as Element).contains(e.relatedTarget as Node)
        ) {
            isDragOver = false;
        }
    }

    function handleDrop(e: DragEvent) {
        e.preventDefault();
        isDragOver = false;

        const droppedFiles = Array.from(e.dataTransfer?.files || []);
        if (droppedFiles.length === 0) return;

        addNotification(`Uploading ${droppedFiles.length} file(s)...`, "info");
        uploadFiles(droppedFiles);
    }

    async function uploadFiles(fileList: File[]) {
        for (const file of fileList) {
            try {
                const formData = new FormData();
                const path = currentPath
                    ? `${currentPath}/${file.name}`
                    : file.name;

                formData.append("path", path);
                formData.append("file", file);

                const res = await fetch("/api/files/upload", {
                    method: "POST",
                    body: formData,
                });

                if (!res.ok) throw new Error(`HTTP ${res.status}`);
                addNotification(
                    `Uploaded '${file.name}' (${formatFileSize(file.size)})`,
                    "info",
                );
            } catch (err) {
                addNotification(
                    `Failed to upload '${file.name}': ${err}`,
                    "error",
                );
            }
        }
        await loadFiles(currentPath);
    }

    function readFileContent(file: File): Promise<string> {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = () => resolve(reader.result as string);
            reader.onerror = () => reject(reader.error);
            reader.readAsText(file);
        });
    }

    async function downloadFile(file: FileInfo) {
        if (file.is_dir) return;

        try {
            const res = await fetch(
                `/api/files/content?path=${encodeURIComponent(file.path)}`,
            );
            if (!res.ok) throw new Error(`HTTP ${res.status}`);

            const data = await res.json();
            const blob = new Blob([data.content], { type: "text/plain" });
            const url = URL.createObjectURL(blob);

            const a = document.createElement("a");
            a.href = url;
            a.download = file.name;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

            addNotification(
                `Downloaded '${file.name}' (${formatFileSize(file.size)})`,
                "info",
            );
        } catch (err) {
            addNotification(
                `Failed to download '${file.name}': ${err}`,
                "error",
            );
        }
    }

    function handleFileClick(file: FileInfo) {
        if (file.is_dir) {
            openFolder(file);
        } else {
            select(file);
        }
    }

    function formatDate(date: string) {
        return new Date(date).toLocaleString();
    }

    $: if (show) loadFiles(currentPath || "");
</script>

{#if show}
    <div
        class="fixed inset-0 bg-black/80 flex items-center justify-center z-50 p-4"
    >
        <div
            class="bg-black border border-gray-600 w-full max-w-5xl h-4/5 flex flex-col font-mono text-sm"
        >
            <!-- Header -->
            <div
                class="flex justify-between items-center p-4 border-b border-gray-600"
            >
                <div class="text-white">
                    <span class="text-green-400">app@minimc</span><span
                        class="text-white">~</span
                    >
                    Files
                    {#if currentPath}
                        <span class="text-gray-400 ml-2">/{currentPath}</span>
                    {/if}
                </div>
                <button
                    on:click={onClose}
                    class="text-red-400 hover:text-red-300 transition-colors"
                >
                    Exit
                </button>
            </div>

            <!-- Create Menu -->
            {#if showCreateMenu}
                <div class="p-4 bg-gray-900/50 border-b border-gray-600">
                    <div class="flex gap-4 items-center text-white">
                        <span class="text-gray-400">new file:</span>
                        <input
                            placeholder="filename.txt"
                            bind:value={newFileName}
                            on:keydown={(e) =>
                                e.key === "Enter" && createFile()}
                            class="px-2 py-1 bg-black border border-gray-600 text-white font-mono outline-none focus:border-green-400"
                        />
                        <button
                            on:click={createFile}
                            class="text-green-400 hover:text-green-300"
                        >
                            Create
                        </button>

                        <span class="text-gray-400 ml-4">new folder:</span>
                        <input
                            placeholder="folder-name"
                            bind:value={newFolderName}
                            on:keydown={(e) =>
                                e.key === "Enter" && createFolder()}
                            class="px-2 py-1 bg-black border border-gray-600 text-white font-mono outline-none focus:border-green-400"
                        />
                        <button
                            on:click={createFolder}
                            class="text-blue-400 hover:text-blue-300"
                        >
                            Create
                        </button>
                    </div>
                </div>
            {/if}

            <!-- Terminal Notifications -->
            {#if notifications.length > 0}
                <div
                    class="border-b border-gray-600 bg-gray-900/80 max-h-32 overflow-y-auto"
                >
                    {#each notifications as notification}
                        <div
                            class="px-4 py-1 text-xs font-mono"
                            class:text-red-400={notification.includes(
                                "[ERROR]",
                            )}
                            class:text-yellow-400={notification.includes(
                                "[WARN]",
                            )}
                            class:text-green-400={notification.includes(
                                "[INFO]",
                            )}
                        >
                            {notification}
                        </div>
                    {/each}
                </div>
            {/if}
            <div class="flex gap-4 p-4 border-b border-gray-600 text-white">
                <button
                    on:click={goBack}
                    disabled={!currentPath}
                    class="text-gray-400 hover:text-white disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                >
                    cd ..
                </button>
                <button
                    on:click={() => (showCreateMenu = !showCreateMenu)}
                    class="text-green-400 hover:text-green-300 transition-colors"
                >
                    + new
                </button>
            </div>

            <!-- Body -->
            <div class="flex flex-1 overflow-hidden">
                <!-- File List -->
                <div class="flex-1 flex flex-col">
                    <div
                        class="flex-1 overflow-y-auto relative"
                        on:dragover={handleDragOver}
                        on:dragleave={handleDragLeave}
                        on:drop={handleDrop}
                        role="region"
                        aria-label="File list and drop zone"
                    >
                        <!-- Drag overlay -->
                        {#if isDragOver}
                            <div
                                class="absolute inset-0 bg-green-900/30 border-2 border-dashed border-green-400 z-10 flex items-center justify-center"
                            >
                                <div
                                    class="text-green-400 text-center font-mono"
                                >
                                    <div class="text-lg">
                                        Drop files here to upload
                                    </div>
                                    <div class="text-sm opacity-70">
                                        Files will be uploaded to /{currentPath}
                                    </div>
                                </div>
                            </div>
                        {/if}
                        {#if loading}
                            <div class="p-4 text-gray-400">Loading...</div>
                        {:else if error}
                            <div class="p-4 text-red-400">Error: {error}</div>
                        {:else if files.length === 0}
                            <div class="p-4 text-gray-400">
                                Empty directory - drag files here to upload
                            </div>
                        {:else}
                            {#each files as file (file.path)}
                                <div
                                    class="flex items-center justify-between px-4 py-2 hover:bg-gray-900/30 cursor-pointer group"
                                    class:bg-gray-900={selectedFile?.path ===
                                        file.path}
                                    on:click={() => handleFileClick(file)}
                                    role="presentation"
                                >
                                    <div
                                        class="flex items-center gap-4 flex-1 min-w-0"
                                    >
                                        <span
                                            class="text-blue-400 w-12 flex-shrink-0"
                                        >
                                            {file.is_dir ? "folder" : "file"}
                                        </span>
                                        <span
                                            class="text-white flex-1 min-w-0 truncate"
                                        >
                                            {file.name}
                                        </span>
                                        {#if isArchive(file.name)}
                                            <span
                                                class="text-yellow-400 text-xs bg-yellow-400/10 px-2 py-1 rounded"
                                            >
                                                ARCHIVE
                                            </span>
                                        {/if}
                                    </div>

                                    <div
                                        class="flex items-center gap-4 opacity-0 group-hover:opacity-100 transition-opacity"
                                    >
                                        {#if !file.is_dir && isArchive(file.name)}
                                            <button
                                                on:click|stopPropagation={() =>
                                                    extractArchive(file)}
                                                class="text-green-400 hover:text-green-300"
                                                title="Extract archive"
                                            >
                                                Extract
                                            </button>
                                        {/if}
                                        {#if !file.is_dir}
                                            <button
                                                on:click|stopPropagation={() =>
                                                    downloadFile(file)}
                                                class="text-blue-400 hover:text-blue-300"
                                            >
                                                Download
                                            </button>
                                        {/if}
                                        <button
                                            class="text-yellow-400 hover:text-yellow-300"
                                        >
                                            Rename
                                        </button>
                                        <button
                                            on:click|stopPropagation={() =>
                                                deleteFile(file)}
                                            class="text-red-400 hover:text-red-300"
                                        >
                                            Delete
                                        </button>
                                    </div>

                                    <div
                                        class="text-gray-400 text-xs ml-4 flex-shrink-0"
                                    >
                                        {#if file.is_dir}
                                            (~{formatFileSize(file.size)})
                                        {:else}
                                            ({formatFileSize(file.size)})
                                        {/if}
                                    </div>
                                </div>
                            {/each}
                        {/if}
                    </div>
                </div>

                <!-- File Content Preview -->
                {#if selectedFile && !selectedFile.is_dir}
                    <div class="w-96 border-l border-gray-600 flex flex-col">
                        <div
                            class="p-4 border-b border-gray-600 flex justify-between items-center"
                        >
                            <div>
                                <div class="text-white">
                                    {selectedFile.name}
                                </div>
                                <div class="text-gray-400 text-xs">
                                    {formatFileSize(selectedFile.size)}
                                </div>
                            </div>
                            <div class="flex gap-2">
                                {#if isEditing}
                                    <button
                                        on:click={saveFile}
                                        class="text-green-400 hover:text-green-300"
                                    >
                                        Save
                                    </button>
                                    <button
                                        on:click={() => (isEditing = false)}
                                        class="text-gray-400 hover:text-white"
                                    >
                                        Cancel
                                    </button>
                                {:else}
                                    <button
                                        on:click={() => (isEditing = true)}
                                        class="text-blue-400 hover:text-blue-300"
                                    >
                                        Edit
                                    </button>
                                {/if}
                            </div>
                        </div>

                        <div class="flex-1 overflow-hidden">
                            {#if isEditing}
                                <textarea
                                    bind:value={fileContent}
                                    class="w-full h-full p-4 bg-black text-white font-mono text-xs resize-none outline-none"
                                    placeholder="File content..."
                                ></textarea>
                            {:else}
                                <pre
                                    class="w-full h-full p-4 bg-black text-white font-mono text-xs overflow-auto whitespace-pre-wrap">{fileContent ||
                                        "Empty file"}</pre>
                            {/if}
                        </div>
                    </div>
                {/if}
            </div>
        </div>
    </div>
{/if}
