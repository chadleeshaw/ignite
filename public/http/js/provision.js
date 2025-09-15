import "https://cdnjs.cloudflare.com/ajax/libs/prism/1.20.0/prism.min.js";
import "https://cdnjs.cloudflare.com/ajax/libs/prism/1.20.0/components/prism-yaml.min.js";
import "https://cdnjs.cloudflare.com/ajax/libs/prism/1.20.0/components/prism-ini.min.js";

// Define custom languages for Prism
Prism.languages.kickstart = {
    'comment': /^\s*#.*/,
    'keyword': /\b(auth|autopart|bootloader|clearpart|device|firewall|install|keyboard|lang|logvol|network|part|raid|reboot|rootpw|skipx|text|timezone|upgrade|xconfig|zerombr)\b/,
    'string': /("|').*?\1/,
    'number': /\b\d+\b/,
    'boolean': /\b(true|false)\b/i,
    'operator': /=/
};

Prism.languages.pxe = {
    'keyword': /\b(?:default|timeout|MENU|LABEL|KERNEL|APPEND)\b/i,
    'string': {
        pattern: /(["'])(?:\\(?:\r\n|[\s\S])|(?!\1)[^\\\r\n])*\1/,
        greedy: true
    },
    'number': /\b\d+\b/,
    'parameter': {
        pattern: /\b\w+\s*=/,
        lookbehind: true,
        inside: {
            'param-name': /\w+/
        }
    },
    'indented': {
        pattern: /^\s+.+?$/m,
        inside: {
            'keyword': /\b(?:LABEL|KERNEL|APPEND)\b/i,
            'string': {
                pattern: /(["'])(?:\\(?:\r\n|[\s\S])|(?!\1)[^\\\r\n])*\1/,
                greedy: true
            },
            'number': /\b\d+\b/,
            'parameter': {
                pattern: /\b\w+\s*=/,
                lookbehind: true,
                inside: {
                    'param-name': /\w+/
                }
            }
        }
    }
};

document.addEventListener("DOMContentLoaded", () => {
    const editableTextarea = document.getElementById("editableTextarea");
    const codeBlock = document.querySelector("#highlightedCode code");
    const highlightedCodeDiv = document.getElementById("highlightedCode");
    const currentFilename = document.getElementById('currentFilename');
    const currentLanguage = document.getElementById('currentLanguage');
    const codeContentInput = document.getElementById('codeContent');

    // Safe setter for element values
    const safeSetElementValue = (elementId, value) => {
        const element = document.getElementById(elementId);
        if (element) element.value = value;
    };

    // Language detection logic
    function detectLanguage(content) {
        if (!content) return 'yaml';
        
        const lines = content.split('\n');
        for (let line of lines) {
            line = line.trim();
            if (line.startsWith('#')) {
                const commentContent = line.slice(1).trim().toLowerCase();
                if (['ks', 'kickstart'].some(keyword => commentContent.includes(keyword))) return 'kickstart';
                if (['yaml', 'yml'].some(keyword => commentContent.includes(keyword))) return 'yaml';
                if (commentContent.includes('ini')) return 'ini';
                if (commentContent.includes('pxe')) return 'pxe';
            } else if (line !== '') {
                if (/^(auth|autopart)\b/.test(line)) return 'kickstart';
                if (line.startsWith('[') || /^[a-zA-Z0-9_]+=/.test(line)) return 'ini';
                if (line.startsWith('default')) return 'pxe';
                break;
            }
        }
        return 'yaml';
    }

    // Highlighting update
    function updateHighlighting() {
        const language = detectLanguage(editableTextarea.value);
        codeBlock.className = `language-${language}`;
        codeBlock.textContent = editableTextarea.value;
        Prism.highlightElement(codeBlock);
        syncScroll();
        updateCurrentLanguage(language);

    }

    function updateCurrentLanguage(language) {
        if (currentLanguage) {
            currentLanguage.textContent = `Language: ${language}`;
            currentLanguage.value = language;
        }
    }

    function updateCurrentFilename(filename) {
        if (currentFilename) {
            currentFilename.textContent = `Filename: ${filename}`;
            currentFilename.value = filename;
        }
    }

    // Scroll synchronization
    let syncScrollScheduled = false;
    function syncScroll() {
        if (!syncScrollScheduled) {
            syncScrollScheduled = true;
            requestAnimationFrame(() => {
                if (highlightedCodeDiv) {
                    highlightedCodeDiv.scrollTop = editableTextarea.scrollTop;
                    highlightedCodeDiv.scrollLeft = editableTextarea.scrollLeft;
                    const lineHeight = parseInt(getComputedStyle(editableTextarea).lineHeight, 10);
                    const lineNumber = Math.floor(editableTextarea.scrollTop / lineHeight);
                    highlightedCodeDiv.scrollTop = lineNumber * lineHeight;
                }
                syncScrollScheduled = false;
            });
        }
    }

    // Debounce function for performance
    const debounce = (func, wait) => {
        let timeout;
        return (...args) => {
            clearTimeout(timeout);
            timeout = setTimeout(() => func.apply(this, args), wait);
        };
    };

    const debouncedUpdateHighlighting = debounce(updateHighlighting, 200);

    // Event handlers
    function handleTab(e) {
        if (e.key === 'Tab') {
            e.preventDefault();
            const start = editableTextarea.selectionStart;
            const end = editableTextarea.selectionEnd;
            editableTextarea.value = editableTextarea.value.substring(0, start) + '    ' + editableTextarea.value.substring(end);
            editableTextarea.selectionStart = editableTextarea.selectionEnd = start + 4;
            updateHighlighting();
        }
    }

    // Event listeners
    editableTextarea.addEventListener('input', updateHighlighting);
    editableTextarea.addEventListener('click', debouncedUpdateHighlighting);
    editableTextarea.addEventListener('scroll', syncScroll);
    editableTextarea.addEventListener('keydown', handleTab);

    document.body.addEventListener('loadFinished', () => {
        updateHighlighting();
        saveEditorState();
    });

    document.body.addEventListener('newFilename', () => {
        const newFilename = currentFilename.textContent.split(': ')[1] || 'untitled';
        updateCurrentFilename(newFilename);
        saveEditorState();
    });
    

    if (highlightedCodeDiv) {
        highlightedCodeDiv.addEventListener('scroll', () => {
            editableTextarea.scrollTop = highlightedCodeDiv.scrollTop;
            editableTextarea.scrollLeft = highlightedCodeDiv.scrollLeft;
        });
    }

    function saveEditorState() {
        if (localStorage) {
            try {
                localStorage.setItem('codeBlockContent', editableTextarea.value);
                if (currentFilename) {
                    const filename = currentFilename.textContent.split(': ')[1] || 'untitled';
                    updateCurrentFilename(filename);
                    localStorage.setItem('currentFilename', filename);
                }
                if (currentLanguage) {
                    localStorage.setItem('currentLanguage', currentLanguage.textContent.split(': ')[1] || 'yaml');
                }
            } catch (error) {
                console.error('Error saving state to localStorage:', error);
                alert('An error occurred while saving the editor state.');
            }
        }
    }
    
    function loadEditorState() {
        if (localStorage) {
            try {
                const savedContent = localStorage.getItem('codeBlockContent');
                const savedFilename = localStorage.getItem('currentFilename');
                const savedLanguage = localStorage.getItem('currentLanguage');
            
                if (savedContent && editableTextarea) {
                    editableTextarea.value = savedContent;
                    updateHighlighting();
                }
                if (savedFilename && currentFilename) {
                    safeSetElementValue('currentFilename', savedFilename);
                    updateCurrentFilename(savedFilename);
                }
                if (savedLanguage && currentLanguage) {
                    safeSetElementValue('currentLanguage', savedLanguage);
                    updateCurrentLanguage(savedLanguage);
                }
            } catch (error) {
                console.error('Error loading state from localStorage:', error);
                alert('An error occurred while loading the editor state.');
            }
        }
    }

    // Clear local storage cache
    function clearSpecificLocalStorage() {
        const keysToClear = [
            'codeBlockContent',
            'currentFilename',
            'currentLanguage'
        ];
    
        try {
            keysToClear.forEach(key => {
                localStorage.removeItem(key);
            });
            alert('Cache was cleared successfully.');
        } catch (error) {
            console.error('Error clearing localStorage:', error);
            alert('An error occurred while clearing the cache.');
        }
    }

    // Event listener for clear cache button
    document.getElementById('clearCacheBtn').addEventListener('click', () => {
        clearSpecificLocalStorage();
    });

    // Event listeners for buttons
    document.getElementById('saveBtn').addEventListener('click', () => {
        if (codeContentInput) {
            codeContentInput.value = editableTextarea.value;
        }
        saveEditorState();
    });

    loadEditorState();
});