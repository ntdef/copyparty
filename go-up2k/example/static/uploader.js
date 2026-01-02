// up2k Client Implementation
class Up2kUploader {
    constructor(baseURL, options = {}) {
        this.baseURL = baseURL;
        this.parallelUploads = options.parallelUploads || 6;
        this.chunkSize = null; // Determined by server
        this.files = [];
        this.paused = false;
    }

    async uploadFile(file) {
        console.log(`Starting upload: ${file.name} (${file.size} bytes)`);

        // Step 1: Hash chunks
        const hashes = await this.hashFile(file);
        console.log(`Hashed ${hashes.length} chunks`);

        // Step 2: Handshake
        const handshake = await this.handshake(file, hashes);
        console.log('Handshake response:', handshake);

        if (handshake.hash.length === 0) {
            console.log('File already uploaded (deduplication)!');
            return { status: 'deduplicated', wark: handshake.wark };
        }

        this.chunkSize = handshake.chunk_size;
        const needHashes = handshake.hash;

        // Step 3: Upload missing chunks
        await this.uploadChunks(file, handshake.wark, needHashes, hashes);

        // Step 4: Verify completion
        const verify = await this.handshake(file, hashes);
        if (verify.hash.length === 0) {
            console.log('Upload complete!');
            return { status: 'complete', wark: handshake.wark };
        } else {
            console.log('Upload incomplete, missing:', verify.hash.length);
            return { status: 'incomplete', missing: verify.hash.length };
        }
    }

    async hashFile(file) {
        const chunkSize = this.calculateChunkSize(file.size);
        const numChunks = Math.ceil(file.size / chunkSize);
        const hashes = [];

        for (let i = 0; i < numChunks; i++) {
            const start = i * chunkSize;
            const end = Math.min(start + chunkSize, file.size);
            const chunk = file.slice(start, end);

            const arrayBuffer = await chunk.arrayBuffer();
            const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
            const hashArray = Array.from(new Uint8Array(hashBuffer));
            const hashHex = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');

            hashes.push(hashHex);

            // Update progress
            const progress = (i + 1) / numChunks * 100;
            this.updateProgress(file.name, progress, 'Hashing');
        }

        return hashes;
    }

    calculateChunkSize(fileSize) {
        const minChunkSize = 1024 * 1024; // 1 MB
        let chunkSize = minChunkSize;
        let stepSize = 512 * 1024;
        let multiplier = 1;

        while (true) {
            const numChunks = Math.ceil(fileSize / chunkSize);
            if (numChunks <= 256) return chunkSize;
            if (chunkSize >= 32 * 1024 * 1024 && numChunks <= 4096) return chunkSize;

            chunkSize += stepSize;
            if (multiplier < 8) {
                multiplier++;
                if (multiplier > 1) stepSize = 512 * 1024 * multiplier;
            }
            if (chunkSize > 32 * 1024 * 1024) return 32 * 1024 * 1024;
        }
    }

    async handshake(file, hashes) {
        const response = await fetch(`${this.baseURL}/upload/handshake`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name: file.name,
                size: file.size,
                lmod: Math.floor(file.lastModified / 1000),
                hash: hashes
            })
        });

        if (!response.ok) throw new Error(`Handshake failed: ${response.statusText}`);
        return response.json();
    }

    async uploadChunks(file, wark, needHashes, allHashes) {
        const chunkSize = this.chunkSize;
        const totalChunks = needHashes.length;
        let uploadedChunks = 0;

        // Upload chunks in parallel (limited by parallelUploads)
        const semaphore = new Semaphore(this.parallelUploads);

        const promises = needHashes.map(async (hash) => {
            await semaphore.acquire();

            try {
                if (this.paused) {
                    await this.waitForResume();
                }

                // Find chunk index
                const chunkIndex = allHashes.indexOf(hash);
                const start = chunkIndex * chunkSize;
                const end = Math.min(start + chunkSize, file.size);
                const chunkBlob = file.slice(start, end);

                // Upload chunk
                await this.uploadChunk(wark, hash, chunkBlob);

                uploadedChunks++;
                const progress = (uploadedChunks / totalChunks) * 100;
                this.updateProgress(file.name, progress, 'Uploading');

            } finally {
                semaphore.release();
            }
        });

        await Promise.all(promises);
    }

    async uploadChunk(wark, hash, chunkBlob) {
        const response = await fetch(`${this.baseURL}/upload/chunk`, {
            method: 'POST',
            headers: {
                'X-Upload-Wark': wark,
                'X-Upload-Hash': hash
            },
            body: chunkBlob
        });

        if (!response.ok) {
            throw new Error(`Chunk upload failed: ${response.statusText}`);
        }

        return response.json();
    }

    pause() {
        this.paused = true;
    }

    resume() {
        this.paused = false;
    }

    waitForResume() {
        return new Promise(resolve => {
            const checkInterval = setInterval(() => {
                if (!this.paused) {
                    clearInterval(checkInterval);
                    resolve();
                }
            }, 100);
        });
    }

    updateProgress(filename, percent, status) {
        const event = new CustomEvent('uploadProgress', {
            detail: { filename, percent, status }
        });
        window.dispatchEvent(event);
    }
}

// Semaphore for limiting concurrency
class Semaphore {
    constructor(max) {
        this.max = max;
        this.count = 0;
        this.queue = [];
    }

    async acquire() {
        if (this.count < this.max) {
            this.count++;
            return;
        }

        return new Promise(resolve => {
            this.queue.push(resolve);
        });
    }

    release() {
        this.count--;
        if (this.queue.length > 0) {
            this.count++;
            const resolve = this.queue.shift();
            resolve();
        }
    }
}

// UI Integration
const uploader = new Up2kUploader('http://localhost:8080');
const dropZone = document.getElementById('dropZone');
const fileInput = document.getElementById('fileInput');
const fileList = document.getElementById('fileList');
const parallelInput = document.getElementById('parallelUploads');
const uploadBtn = document.getElementById('uploadBtn');
const pauseBtn = document.getElementById('pauseBtn');

let selectedFiles = [];

dropZone.addEventListener('click', () => fileInput.click());
dropZone.addEventListener('dragover', (e) => {
    e.preventDefault();
    dropZone.classList.add('dragover');
});
dropZone.addEventListener('dragleave', () => dropZone.classList.remove('dragover'));
dropZone.addEventListener('drop', (e) => {
    e.preventDefault();
    dropZone.classList.remove('dragover');
    selectedFiles = Array.from(e.dataTransfer.files);
    displayFiles();
});

fileInput.addEventListener('change', (e) => {
    selectedFiles = Array.from(e.target.files);
    displayFiles();
});

parallelInput.addEventListener('change', (e) => {
    uploader.parallelUploads = parseInt(e.target.value);
});

uploadBtn.addEventListener('click', async () => {
    for (const file of selectedFiles) {
        try {
            await uploader.uploadFile(file);
        } catch (error) {
            console.error('Upload failed:', error);
        }
    }
});

pauseBtn.addEventListener('click', () => {
    if (uploader.paused) {
        uploader.resume();
        pauseBtn.textContent = 'Pause';
    } else {
        uploader.pause();
        pauseBtn.textContent = 'Resume';
    }
});

window.addEventListener('uploadProgress', (e) => {
    const { filename, percent, status } = e.detail;
    updateFileProgress(filename, percent, status);
});

function displayFiles() {
    fileList.innerHTML = selectedFiles.map(file => `
        <div class="file-item" data-filename="${file.name}">
            <strong>${file.name}</strong> (${formatBytes(file.size)})
            <div class="progress-bar">
                <div class="progress-fill" style="width: 0%"></div>
            </div>
            <div class="status">Ready</div>
        </div>
    `).join('');
}

function updateFileProgress(filename, percent, status) {
    const item = document.querySelector(`[data-filename="${filename}"]`);
    if (item) {
        const fill = item.querySelector('.progress-fill');
        const statusDiv = item.querySelector('.status');
        fill.style.width = `${percent}%`;
        statusDiv.textContent = `${status}: ${Math.round(percent)}%`;
    }
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}
