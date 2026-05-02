// frontend/src/App.tsx
import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import "./App.css";
import {
  User,
  Copy,
  Check,
  Send,
  Paperclip,
  UserPlus,
  X,
  Download,
  CheckCircle2,
  ClipboardPaste,
  Link,
  Braces,
  Share2,
  Edit3,
  Key,
  Hash,
  QrCode,
  Fingerprint,
  Minimize2,
} from "lucide-react";
import { storage } from "../wailsjs/go/models";
import FileArt from "./FileArtGenerator";
import {
  GetProfile,
  GetContacts,
  GetMessages,
  SendMessage,
  AddContact,
  SwitchRoom,
  SendFileNative,
  DownloadAndSaveFile,
  SetWindowActive,
  GetMyContactLink,
  GetMyContactJSON,
  UpdateNickname,
  UpdateContactNickname,
} from "../wailsjs/go/main/App";
import { EventsOn, Quit, WindowMinimise } from "../wailsjs/runtime/runtime";

// ============================================================
// TYPES
// ============================================================
interface Profile {
  nickname: string;
  hash: string;
  publicKey: string;
  ed25519PublicKey?: string;
  seedPhrase: string;
}

interface NewMessageEvent {
  roomHash: string;
  direction: string;
  text: string;
  timestamp: number;
  sender: string;
}

interface UIMessage {
  id: string;
  content: string;
  sender: "user" | "contact";
  timestamp: Date;
  isFile?: boolean;
  fileInfo?: FileAttachment;
  status?: "sent" | "delivered" | "read";
}

interface FileAttachment {
  fileName: string;
  fileHash: string;
  fileSize?: string;
}

interface ContactPayload {
  hash: string;
  x25519: string;
  ed25519?: string;
  nickname: string;
}

interface ImportErrorEvent {
  error: string;
}

interface UploadStartedEvent {
  fileName: string;
}

interface UploadProgressEvent {
  status: string;
  progress: number;
}

interface UploadCompletedEvent {
  fileName: string;
  fileHash: string;
  fileSize?: number;
}

interface UploadErrorEvent {
  error: string;
}

interface DownloadErrorEvent {
  error: string;
}

// ============================================================
// CONSTANTS
// ============================================================
const MAX_PROCESSED_IDS = 1000;
const MESSAGE_FETCH_LIMIT = 50;
const COPY_TIMEOUT_MS = 2000;
const FILE_PATTERN = / File: (.+)\n(?:Size: (.+)\n)?Hash: ([a-f0-9]+)/;
const HASH_HEX_PATTERN = /^[0-9a-fA-F]+$/;
const HASH_LENGTH = 64;

// ============================================================
// UTILITY FUNCTIONS
// ============================================================
const truncateHash = (hash: string, length = 8): string => {
  if (!hash) return "";
  if (hash.length <= length * 2) return hash;
  return `${hash.slice(0, length)}...${hash.slice(-length)}`;
};

const humanizeBytes = (bytes: number): string => {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`;
  return `${(bytes / 1073741824).toFixed(1)} GB`;
};

const getAvatarUrl = (hash: string): string =>
  `https://api.dicebear.com/7.x/avataaars/svg?seed=${hash}`;

const parseFileMessage = (content: string): FileAttachment | null => {
  const match = content.match(FILE_PATTERN);
  return match
    ? {
        fileName: match[1],
        fileSize: match[2] || undefined,
        fileHash: match[3],
      }
    : null;
};

const generateContactLink = (contact: ContactPayload): string => {
  const params = new URLSearchParams({
    hash: contact.hash,
    x25519: contact.x25519,
    nickname: contact.nickname,
  });
  if (contact.ed25519) params.set("ed25519", contact.ed25519);
  return `lastchance://contact?${params}`;
};

const parseContactInput = (input: string): ContactPayload | null => {
  const trimmed = input.trim();

  // Try URL format
  if (trimmed.startsWith("lastchance://contact?")) {
    try {
      const url = new URL(trimmed);
      const params = url.searchParams;
      const hash = params.get("hash") || "";
      const x25519 = params.get("x25519") || "";
      const ed25519 = params.get("ed25519") || undefined;
      const nickname = params.get("nickname") || "";

      if (hash && x25519 && nickname) {
        return { hash, x25519, ed25519, nickname };
      }
    } catch {}
  }

  // Try JSON format
  if (trimmed.startsWith("{")) {
    try {
      const obj = JSON.parse(trimmed);
      if (obj.hash && obj.x25519 && obj.nickname) {
        return {
          hash: obj.hash,
          x25519: obj.x25519,
          ed25519: obj.ed25519,
          nickname: obj.nickname,
        };
      }
    } catch {}
  }

  return null;
};

// ============================================================
// APP COMPONENT
// ============================================================
function App() {
  // ---------- State ----------
  const [profile, setProfile] = useState<Profile | null>(null);
  const [contacts, setContacts] = useState<storage.Contact[]>([]);
  const [selectedId, setSelectedId] = useState<string>("");
  const [messages, setMessages] = useState<Record<string, UIMessage[]>>({});
  const [loading, setLoading] = useState(true);
  const [copiedField, setCopiedField] = useState<string | null>(null);
  const [inputValue, setInputValue] = useState("");
  const [showAddContact, setShowAddContact] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadStatus, setUploadStatus] = useState("");
  const [unreadCounts, setUnreadCounts] = useState<Record<string, number>>({});
  const [importedContact, setImportedContact] = useState<ContactPayload | null>(
    null,
  );
  const [sidebarExpanded, setSidebarExpanded] = useState(true);
  const [showProfileModal, setShowProfileModal] = useState(false);
  const [showShareModal, setShowShareModal] = useState(false);
  const [renamingHash, setRenamingHash] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");

  // ---------- Refs ----------
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const processedMessageIds = useRef<Set<string>>(new Set());

  // ---------- Memoized values ----------
  const selectedContact = useMemo(
    () => contacts.find((c) => c.hash === selectedId),
    [contacts, selectedId],
  );

  const currentMessages = useMemo(
    () => messages[selectedId] || [],
    [messages, selectedId],
  );

  // ---------- Callbacks ----------
  const copyToClipboard = useCallback((text: string, field: string) => {
    navigator.clipboard.writeText(text);
    setCopiedField(field);
    setTimeout(() => setCopiedField(null), COPY_TIMEOUT_MS);
  }, []);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  // ---------- Window controls ----------
  const handleMinimize = useCallback(() => {
    try {
      WindowMinimise();
    } catch (error) {
      console.error("Minimize failed:", error);
    }
  }, []);

  const handleClose = useCallback(() => {
    try {
      Quit();
    } catch (error) {
      console.error("Close failed:", error);
    }
  }, []);

  // ---------- Data loading ----------
  const loadInitialData = useCallback(async () => {
    try {
      const prof = await GetProfile();
      setProfile(prof as Profile);

      const conts = await GetContacts();
      setContacts(conts);

      if (conts.length > 0 && !selectedId) {
        setSelectedId(conts[0].hash);
      }
    } catch (error) {
      console.error("Failed to load initial data:", error);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadMessagesForContact = useCallback(async (hash: string) => {
    try {
      const msgs = await GetMessages(hash, MESSAGE_FETCH_LIMIT);
      const ui = msgs.map((m: storage.Message) => ({
        id: String(m.id),
        content: m.text,
        sender: (m.direction === "out" ? "user" : "contact") as
          | "user"
          | "contact",
        timestamp: new Date(m.timestamp),
        isFile: !!parseFileMessage(m.text),
        fileInfo: parseFileMessage(m.text) || undefined,
      }));
      setMessages((prev) => ({ ...prev, [hash]: ui }));
    } catch (error) {
      console.error("Failed to load messages:", error);
    }
  }, []);

  // ---------- Message handling ----------
  const handleNewMessage = useCallback(
    (event: NewMessageEvent) => {
      if (event.sender?.includes("(self)") || !event.roomHash) return;

      const dedupKey = `${event.roomHash}:${event.text}:${event.timestamp}`;
      if (processedMessageIds.current.has(dedupKey)) return;

      processedMessageIds.current.add(dedupKey);
      if (processedMessageIds.current.size > MAX_PROCESSED_IDS) {
        processedMessageIds.current.clear();
      }

      const fileInfo = parseFileMessage(event.text);
      const newMessage: UIMessage = {
        id: `msg_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
        content: event.text,
        sender: "contact",
        timestamp: new Date(event.timestamp),
        isFile: !!fileInfo,
        fileInfo: fileInfo || undefined,
      };

      setMessages((prev) => ({
        ...prev,
        [event.roomHash]: [...(prev[event.roomHash] || []), newMessage],
      }));

      GetContacts().then(setContacts);

      if (event.roomHash !== selectedId) {
        setUnreadCounts((prev) => ({
          ...prev,
          [event.roomHash]: (prev[event.roomHash] || 0) + 1,
        }));
      }
    },
    [selectedId],
  );

  const handleSend = useCallback(
    async (e?: React.FormEvent) => {
      if (e) e.preventDefault();
      if (!inputValue.trim() || !selectedId || !profile) return;

      const content = inputValue.trim();
      setInputValue("");

      try {
        await SendMessage(selectedId, content);
        const fileInfo = parseFileMessage(content);

        setMessages((prev) => ({
          ...prev,
          [selectedId]: [
            ...(prev[selectedId] || []),
            {
              id: `msg_${Date.now()}`,
              content,
              sender: "user",
              timestamp: new Date(),
              isFile: !!fileInfo,
              fileInfo: fileInfo || undefined,
            },
          ],
        }));
      } catch (error) {
        console.error("Send message failed:", error);
      }
    },
    [inputValue, selectedId, profile],
  );

  // ---------- File handling ----------
  const handleAttachClick = useCallback(async () => {
    if (!selectedId) {
      alert("Выберите чат");
      return;
    }
    setIsUploading(true);
    try {
      await SendFileNative(selectedId);
    } catch (error: any) {
      alert(error.message);
      setIsUploading(false);
      setUploadStatus("");
    }
  }, [selectedId]);

  const handleFileDownload = useCallback(async (fileInfo: FileAttachment) => {
    if (!fileInfo.fileHash) return;
    try {
      await DownloadAndSaveFile(fileInfo.fileHash, fileInfo.fileName);
    } catch (error: any) {
      alert(error.message);
    }
  }, []);

  // ---------- Contact management ----------
  const handleAddContact = useCallback(
    async (hash: string, pk: string, nick: string) => {
      await AddContact(hash, pk, nick);
      const conts = await GetContacts();
      setContacts(conts);
      if (conts.length === 1) setSelectedId(conts[0].hash);
    },
    [],
  );

  const handleUpdateOwnNickname = useCallback(async (newNick: string) => {
    if (!newNick.trim()) return;
    try {
      await UpdateNickname(newNick.trim());
      setProfile((prev) =>
        prev ? { ...prev, nickname: newNick.trim() } : null,
      );
    } catch (error) {
      console.error("Update nickname failed:", error);
      throw error;
    }
  }, []);

  const handleContactRename = useCallback(
    async (hash: string) => {
      const newName = renameValue.trim();
      if (!newName) return;
      try {
        await UpdateContactNickname(hash, newName);
        const conts = await GetContacts();
        setContacts(conts);
      } catch (error: any) {
        alert(error.message);
      } finally {
        setRenamingHash(null);
      }
    },
    [renameValue],
  );

  // ---------- Effects ----------
  useEffect(() => {
    loadInitialData();
  }, [loadInitialData]);

  useEffect(() => {
    if (!selectedId) return;
    loadMessagesForContact(selectedId);
    SwitchRoom(selectedId);
    setUnreadCounts((prev) =>
      prev[selectedId] === 0 ? prev : { ...prev, [selectedId]: 0 },
    );
  }, [selectedId, loadMessagesForContact]);

  useEffect(() => {
    scrollToBottom();
  }, [currentMessages, scrollToBottom]);

  // Window focus management
  useEffect(() => {
    const handleFocus = () => {
      SetWindowActive(true);
      if (selectedId) {
        setUnreadCounts((prev) => ({ ...prev, [selectedId]: 0 }));
      }
    };

    const handleBlur = () => SetWindowActive(false);

    SetWindowActive(document.hasFocus());
    window.addEventListener("focus", handleFocus);
    window.addEventListener("blur", handleBlur);

    return () => {
      window.removeEventListener("focus", handleFocus);
      window.removeEventListener("blur", handleBlur);
    };
  }, [selectedId]);

  // Global event listeners
  useEffect(() => {
    const unsubscribers: (() => void)[] = [];

    unsubscribers.push(EventsOn("new-message", handleNewMessage));

    unsubscribers.push(
      EventsOn("contact-import", (data: ContactPayload) => {
        setImportedContact(data);
        setShowAddContact(true);
      }),
    );

    unsubscribers.push(
      EventsOn("contact-import-error", (data: ImportErrorEvent) => {
        console.error(data.error);
      }),
    );

    unsubscribers.push(
      EventsOn("upload-started", (data: UploadStartedEvent) => {
        setIsUploading(true);
        setUploadStatus(`Отправка ${data.fileName}...`);
      }),
    );

    unsubscribers.push(
      EventsOn("upload-progress", (data: UploadProgressEvent) => {
        setUploadStatus(`${data.status} (${data.progress}%)`);
      }),
    );

    unsubscribers.push(
      EventsOn("upload-completed", (data: UploadCompletedEvent) => {
        setIsUploading(false);
        setUploadStatus("");

        const msgText = ` File: ${data.fileName}\nSize: ${
          data.fileSize ? humanizeBytes(data.fileSize) : "unknown"
        }\nHash: ${data.fileHash}`;

        setMessages((prev) => ({
          ...prev,
          [selectedId]: [
            ...(prev[selectedId] || []),
            {
              id: `msg_${Date.now()}`,
              content: msgText,
              sender: "user",
              timestamp: new Date(),
              isFile: true,
              fileInfo: {
                fileName: data.fileName,
                fileHash: data.fileHash,
                fileSize: data.fileSize
                  ? humanizeBytes(data.fileSize)
                  : undefined,
              },
            },
          ],
        }));
      }),
    );

    unsubscribers.push(
      EventsOn("upload-error", (data: UploadErrorEvent) => {
        setIsUploading(false);
        setUploadStatus("");
        alert(data.error);
      }),
    );

    unsubscribers.push(
      EventsOn("download-started", () => setUploadStatus("Download...")),
    );

    unsubscribers.push(
      EventsOn("download-completed", () => setUploadStatus("")),
    );

    unsubscribers.push(
      EventsOn("download-error", (data: DownloadErrorEvent) => {
        setUploadStatus("");
        alert(data.error);
      }),
    );

    return () => {
      unsubscribers.forEach((unsub) => unsub());
    };
  }, [handleNewMessage, selectedId]);

  // ---------- Render helpers ----------
  const renderMessageContent = useCallback(
    (msg: UIMessage) => {
      if (msg.isFile && msg.fileInfo) {
        const isOwn = msg.sender === "user";
        return (
          <div
            className={`file-message-card ${isOwn ? "own-file" : "contact-file"}`}
          >
            <FileArt
              fileHash={msg.fileInfo.fileHash}
              fileName={msg.fileInfo.fileName}
              size={100}
              onClick={
                isOwn ? undefined : () => handleFileDownload(msg.fileInfo!)
              }
            />
            <div className="file-message-info">
              <div className="file-message-name" title={msg.fileInfo.fileName}>
                {msg.fileInfo.fileName.length > 30
                  ? `${msg.fileInfo.fileName.slice(0, 27)}...`
                  : msg.fileInfo.fileName}
              </div>
              {msg.fileInfo.fileSize && (
                <div className="file-message-size">{msg.fileInfo.fileSize}</div>
              )}
              <div className="file-message-hash-row">
                <code className="file-hash">
                  {truncateHash(msg.fileInfo.fileHash, 6)}
                </code>
                <button
                  className="copy-hash-btn"
                  onClick={(e) => {
                    e.stopPropagation();
                    copyToClipboard(
                      msg.fileInfo!.fileHash,
                      `f-${msg.fileInfo!.fileHash}`,
                    );
                  }}
                >
                  {copiedField === `f-${msg.fileInfo!.fileHash}` ? (
                    <Check size={12} className="text-success" />
                  ) : (
                    <Copy size={12} />
                  )}
                </button>
              </div>
              {isOwn ? (
                <div className="file-status-sent">
                  <CheckCircle2 size={14} />
                  <span>File sent</span>
                </div>
              ) : (
                <button
                  className="download-file-btn-full"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleFileDownload(msg.fileInfo!);
                  }}
                >
                  <Download size={14} />
                  <span>Download file</span>
                </button>
              )}
            </div>
          </div>
        );
      }
      return <span className="message-text">{msg.content}</span>;
    },
    [copiedField, copyToClipboard, handleFileDownload],
  );

  // ---------- Loading state ----------
  if (loading) {
    return (
      <div className="loading-screen">
        <div className="loading-spinner" />
        <p>Loading LastChance...</p>
      </div>
    );
  }

  // ---------- Main render ----------
  return (
    <>
      {/* ---- Custom Frameless Header ---- */}
      <header className="custom-header">
        <div className="drag-zone">
          <span className="app-title">LastChance Messenger</span>
        </div>
        <div className="window-controls">
          <button
            onClick={handleMinimize}
            className="minimize-btn"
            aria-label="Minimize"
          >
            <Minimize2 size={14} />
          </button>
          <button
            onClick={handleClose}
            className="close-btn"
            aria-label="Close"
          >
            <X size={14} />
          </button>
        </div>
      </header>

      {/* ---- Main App Container ---- */}
      <div className="app-container-new">
        {/* Upload status bar */}
        {uploadStatus && (
          <div className="upload-status-bar">
            <span>{uploadStatus}</span>
            {isUploading && <div className="upload-spinner" />}
          </div>
        )}

        {/* ---- Sidebar ---- */}
        <aside className={`sidebar-new ${!sidebarExpanded ? "collapsed" : ""}`}>
          {/* Control Bubble */}
          <div className="sidebar-bubble bubble-controls">
            <div className="sidebar-header">
              <div
                className={`burger-wrapper ${sidebarExpanded ? "active" : ""}`}
                onClick={() => setSidebarExpanded(!sidebarExpanded)}
              >
                <button className="burger-btn" aria-label="Toggle Sidebar">
                  <span />
                  <span />
                  <span />
                </button>
              </div>

              <div className="sidebar-actions">
                <button
                  className="profile-action-btn"
                  onClick={() => setShowProfileModal(true)}
                  title="View Profile"
                >
                  <User size={18} />
                  <span className="action-text">Profile</span>
                </button>
                <button
                  className="profile-action-btn"
                  onClick={() => setShowShareModal(true)}
                  title="Share Contact"
                >
                  <Share2 size={18} />
                  <span className="action-text">Share</span>
                </button>
              </div>
            </div>
          </div>

          {/* Contacts Bubble */}
          <div className="sidebar-bubble bubble-contacts">
            <div className="contacts-card">
              {sidebarExpanded && (
                <div className="contacts-header-row">
                  <h4 className="contacts-title">Contacts</h4>
                  <button
                    className="add-contact-icon-btn"
                    onClick={() => {
                      setImportedContact(null);
                      setShowAddContact(true);
                    }}
                  >
                    <UserPlus size={16} />
                  </button>
                </div>
              )}

              <div className="contacts-list">
                {contacts.length === 0 ? (
                  <p className="no-contacts">
                    {sidebarExpanded ? "No contacts" : ""}
                  </p>
                ) : (
                  contacts.map((contact) => (
                    <div
                      key={contact.hash}
                      className={`contact-item ${
                        selectedId === contact.hash ? "active" : ""
                      } ${!sidebarExpanded ? "rail-item" : ""}`}
                      onClick={() => {
                        setSelectedId(contact.hash);
                        setUnreadCounts((prev) => ({
                          ...prev,
                          [contact.hash]: 0,
                        }));
                      }}
                    >
                      <img
                        src={getAvatarUrl(contact.hash)}
                        alt={contact.nickname}
                        className={`contact-avatar ${
                          !sidebarExpanded ? "contact-avatar-small" : ""
                        }`}
                      />
                      {sidebarExpanded && (
                        <>
                          <div className="contact-details">
                            {renamingHash === contact.hash ? (
                              <div className="contact-rename-inline">
                                <input
                                  value={renameValue}
                                  onChange={(e) =>
                                    setRenameValue(e.target.value)
                                  }
                                  onKeyDown={(e) => {
                                    if (e.key === "Enter") {
                                      handleContactRename(contact.hash);
                                    }
                                  }}
                                  onBlur={() =>
                                    handleContactRename(contact.hash)
                                  }
                                  autoFocus
                                />
                              </div>
                            ) : (
                              <span className="contact-name">
                                {contact.nickname}
                              </span>
                            )}
                            <span className="contact-hash">
                              {truncateHash(contact.hash, 4)}
                            </span>
                          </div>
                          <button
                            className="contact-rename-btn"
                            onClick={(e) => {
                              e.stopPropagation();
                              if (renamingHash === contact.hash) {
                                setRenamingHash(null);
                              } else {
                                setRenamingHash(contact.hash);
                                setRenameValue(contact.nickname);
                              }
                            }}
                            title="Rename contact"
                          >
                            <Edit3 size={12} />
                          </button>
                        </>
                      )}
                      {unreadCounts[contact.hash] > 0 && (
                        <span className="unread-badge rail-unread">
                          {unreadCounts[contact.hash] > 99
                            ? "99+"
                            : unreadCounts[contact.hash]}
                        </span>
                      )}
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
        </aside>

        {/* ---- Chat Area ---- */}
        <div className="chat-area-new">
          {selectedContact ? (
            <>
              <div className="chat-header-new glass-card">
                <div className="chat-contact-info">
                  <div className="avatar-small">
                    <User size={20} />
                  </div>
                  <div>
                    <h3>{selectedContact.nickname}</h3>
                    <span className="contact-hash-header">
                      {truncateHash(selectedContact.hash, 8)}
                    </span>
                  </div>
                </div>
              </div>

              <div
                className="messages-container-new"
                ref={messagesContainerRef}
              >
                {currentMessages.length === 0 ? (
                  <div className="no-messages">
                    <p>No messages yet</p>
                    <p className="hint">Send a message</p>
                  </div>
                ) : (
                  currentMessages.map((msg) => (
                    <div
                      key={msg.id}
                      className={`message-wrapper ${
                        msg.sender === "user" ? "own" : "other"
                      } message-enter`}
                    >
                      <div
                        className={`message-bubble ${
                          msg.sender === "user" ? "own" : "other"
                        } ${msg.isFile ? "file-message-bubble" : ""} glass-bubble`}
                      >
                        {renderMessageContent(msg)}
                        <div className="message-meta">
                          {msg.sender === "user" ? (
                            <MessageStatus
                              timestamp={msg.timestamp}
                              status={msg.status || "read"}
                            />
                          ) : (
                            <span className="time">
                              {msg.timestamp.toLocaleTimeString([], {
                                hour: "2-digit",
                                minute: "2-digit",
                              })}
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                  ))
                )}
                <div ref={messagesEndRef} />
              </div>

              <form
                className="message-input-form-new glass-card"
                onSubmit={handleSend}
              >
                <button
                  type="button"
                  className="attach-btn"
                  onClick={handleAttachClick}
                  disabled={isUploading}
                >
                  <Paperclip size={20} />
                </button>
                <input
                  type="text"
                  value={inputValue}
                  onChange={(e) => setInputValue(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      handleSend();
                    }
                  }}
                  placeholder="Type a message..."
                  className="message-input-new"
                  disabled={isUploading}
                />
                <button
                  type="submit"
                  className="send-btn"
                  disabled={!inputValue.trim() || isUploading}
                >
                  <Send size={20} />
                </button>
              </form>
            </>
          ) : (
            <div className="no-chat-selected">
              <User size={64} />
              <h2>No contact selected</h2>
              <p>
                {contacts.length > 0 ? "Select a contact" : "Add a contact"}
              </p>
            </div>
          )}
        </div>

        {/* ---- Modals ---- */}
        {showAddContact && (
          <AddContactModal
            onClose={() => {
              setShowAddContact(false);
              setImportedContact(null);
            }}
            onAdd={handleAddContact}
            importedContact={importedContact}
          />
        )}

        {showProfileModal && profile && (
          <ProfileModal
            profile={profile}
            onClose={() => setShowProfileModal(false)}
            copyToClipboard={copyToClipboard}
            copiedField={copiedField}
            onUpdateNickname={handleUpdateOwnNickname}
          />
        )}

        {showShareModal && (
          <ShareModal
            onClose={() => setShowShareModal(false)}
            copyToClipboard={copyToClipboard}
            copiedField={copiedField}
          />
        )}
      </div>
    </>
  );
}

// ============================================================
// SUB-COMPONENTS
// ============================================================

// ---------- Profile Modal ----------
const ProfileModal = ({
  profile,
  onClose,
  copyToClipboard,
  copiedField,
  onUpdateNickname,
}: {
  profile: Profile;
  onClose: () => void;
  copyToClipboard: (text: string, field: string) => void;
  copiedField: string | null;
  onUpdateNickname: (newNick: string) => Promise<void>;
}) => {
  const [editing, setEditing] = useState(false);
  const [nickInput, setNickInput] = useState(profile.nickname);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    if (!nickInput.trim() || nickInput === profile.nickname) {
      setEditing(false);
      return;
    }
    setSaving(true);
    try {
      await onUpdateNickname(nickInput.trim());
      setEditing(false);
    } catch {
      alert("Failed to update nickname");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal modal-wide glass-card"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <h3>Your Cryptographic Profile</h3>
          <button className="modal-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>
        <div className="profile-data">
          {/* Nickname field */}
          <div className="profile-field">
            <User size={16} />
            <strong>Nickname:</strong>
            {editing ? (
              <div className="nickname-edit-row">
                <input
                  value={nickInput}
                  onChange={(e) => setNickInput(e.target.value)}
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleSave();
                    if (e.key === "Escape") setEditing(false);
                  }}
                />
                <button
                  className="nickname-save-btn"
                  onClick={handleSave}
                  disabled={saving}
                >
                  {saving ? "Saving..." : "Save"}
                </button>
                <button
                  className="nickname-cancel-btn"
                  onClick={() => {
                    setEditing(false);
                    setNickInput(profile.nickname);
                  }}
                >
                  Cancel
                </button>
              </div>
            ) : (
              <>
                <span>{profile.nickname}</span>
                <button
                  className="edit-nickname-btn"
                  onClick={() => setEditing(true)}
                >
                  <Edit3 size={14} />
                </button>
              </>
            )}
          </div>

          {/* Hash field */}
          <div className="profile-field">
            <Hash size={16} />
            <strong>Hash:</strong>
            <code className="mono">
              {profile.hash}
              <button
                className="copy-btn-inline"
                onClick={() => copyToClipboard(profile.hash, "hash")}
              >
                {copiedField === "hash" ? (
                  <Check size={14} />
                ) : (
                  <Copy size={14} />
                )}
              </button>
            </code>
          </div>

          {/* X25519 public key */}
          <div className="profile-field">
            <Key size={16} />
            <strong>X25519 Public Key:</strong>
            <code className="mono">
              {profile.publicKey}
              <button
                className="copy-btn-inline"
                onClick={() => copyToClipboard(profile.publicKey, "pubkey")}
              >
                {copiedField === "pubkey" ? (
                  <Check size={14} />
                ) : (
                  <Copy size={14} />
                )}
              </button>
            </code>
          </div>

          {/* Ed25519 public key (optional) */}
          {profile.ed25519PublicKey && (
            <div className="profile-field">
              <Fingerprint size={16} />
              <strong>Ed25519 Public Key:</strong>
              <code className="mono">
                {profile.ed25519PublicKey}
                <button
                  className="copy-btn-inline"
                  onClick={() =>
                    copyToClipboard(profile.ed25519PublicKey!, "ed25519")
                  }
                >
                  {copiedField === "ed25519" ? (
                    <Check size={14} />
                  ) : (
                    <Copy size={14} />
                  )}
                </button>
              </code>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

// ---------- Share Modal ----------
const ShareModal = ({
  onClose,
  copyToClipboard,
  copiedField,
}: {
  onClose: () => void;
  copyToClipboard: (text: string, field: string) => void;
  copiedField: string | null;
}) => {
  const handleCopyLink = async () => {
    const link = await GetMyContactLink();
    copyToClipboard(link, "share-link");
  };

  const handleCopyJSON = async () => {
    const json = await GetMyContactJSON();
    copyToClipboard(json, "share-json");
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal modal-wide glass-card"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <h3>Share Your Contact</h3>
          <button className="modal-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="share-cards">
          {/* Copy link card */}
          <div className="share-card" onClick={handleCopyLink}>
            <div className="share-card-icon">
              <Link size={24} />
            </div>
            <div className="share-card-content">
              <div className="share-card-title">Copy Invite Link</div>
              <div className="share-card-desc">lastchance://contact?...</div>
            </div>
            {copiedField === "share-link" ? (
              <Check size={20} style={{ color: "#48bb78" }} />
            ) : (
              <Copy size={20} style={{ color: "#808080" }} />
            )}
          </div>

          {/* Copy JSON card */}
          <div className="share-card" onClick={handleCopyJSON}>
            <div className="share-card-icon">
              <Braces size={24} />
            </div>
            <div className="share-card-content">
              <div className="share-card-title">Copy JSON Identity</div>
              <div className="share-card-desc">
                Full public keys &amp; nickname
              </div>
            </div>
            {copiedField === "share-json" ? (
              <Check size={20} style={{ color: "#48bb78" }} />
            ) : (
              <Copy size={20} style={{ color: "#808080" }} />
            )}
          </div>
        </div>

        {/* QR Placeholder */}
        <div className="share-qr-placeholder">
          <QrCode size={32} />
          <span>QR Code coming soon</span>
        </div>
      </div>
    </div>
  );
};

// ---------- Add Contact Modal ----------
function AddContactModal({
  onClose,
  onAdd,
  importedContact,
}: {
  onClose: () => void;
  onAdd: (hash: string, x25519: string, nickname: string) => Promise<void>;
  importedContact: ContactPayload | null;
}) {
  const [input, setInput] = useState("");
  const [parsed, setParsed] = useState<ContactPayload | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<"auto" | "manual">("auto");
  const [manualHash, setManualHash] = useState("");
  const [manualX25519, setManualX25519] = useState("");
  const [manualNick, setManualNick] = useState("");

  useEffect(() => {
    if (importedContact) {
      setInput(generateContactLink(importedContact));
    }
  }, [importedContact]);

  useEffect(() => {
    if (!input.trim()) {
      setParsed(null);
      setMode("auto");
      return;
    }

    const result = parseContactInput(input.trim());
    if (result) {
      setParsed(result);
      setError("");
      setMode("auto");
    } else {
      setParsed(null);
      setMode("manual");

      const parts = input.trim().split(/\s+/);
      if (
        parts.length >= 2 &&
        parts[0].length === HASH_LENGTH &&
        parts[1].length === HASH_LENGTH
      ) {
        setManualHash(parts[0]);
        setManualX25519(parts[1]);
        setManualNick(parts.slice(2).join(" "));
      }
    }
  }, [input]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      let hash: string, x25519: string, nickname: string;

      if (mode === "auto" && parsed) {
        hash = parsed.hash;
        x25519 = parsed.x25519;
        nickname = parsed.nickname;
      } else {
        hash = manualHash;
        x25519 = manualX25519;
        nickname = manualNick;
      }

      // Validation
      if (!nickname.trim()) throw new Error("Введите никнейм");
      if (!hash || hash.length !== HASH_LENGTH) throw new Error("Hash: 64 hex");
      if (!x25519 || x25519.length !== HASH_LENGTH)
        throw new Error("X25519: 64 hex");
      if (!HASH_HEX_PATTERN.test(hash)) throw new Error("Hash: hex");
      if (!HASH_HEX_PATTERN.test(x25519)) throw new Error("Ключ: hex");

      await onAdd(hash, x25519, nickname.trim());
      onClose();
    } catch (error: any) {
      setError(error.message);
    } finally {
      setLoading(false);
    }
  };

  const handlePaste = async () => {
    try {
      setInput(await navigator.clipboard.readText());
    } catch {
      setError("Буфер недоступен");
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal modal-wide glass-card"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <h3>Add a contact</h3>
          <button className="modal-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>Paste link, JSON or enter manually</label>
            <div className="input-with-paste">
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="lastchance://contact?hash=...&x25519=...&nickname=..."
                className="contact-input-area"
                rows={4}
                autoFocus
              />
              <button type="button" className="paste-btn" onClick={handlePaste}>
                <ClipboardPaste size={16} />
              </button>
            </div>
          </div>

          {mode === "auto" && parsed && (
            <div className="contact-preview glass-card">
              <div className="contact-preview-avatar">
                <User size={32} />
              </div>
              <div className="contact-preview-info">
                <strong>{parsed.nickname}</strong>
                <code className="contact-preview-hash">
                  {truncateHash(parsed.hash, 8)}
                </code>
                {parsed.ed25519 && (
                  <code className="contact-preview-key">
                    Ed25519: {truncateHash(parsed.ed25519, 8)}
                  </code>
                )}
              </div>
              <CheckCircle2 size={20} style={{ color: "#48bb78" }} />
            </div>
          )}

          {mode === "manual" && (
            <>
              <div className="form-group">
                <label>Nickname</label>
                <input
                  value={manualNick}
                  onChange={(e) => setManualNick(e.target.value)}
                  placeholder="Имя"
                  required
                />
              </div>
              <div className="form-group">
                <label>Hash (64 hex)</label>
                <input
                  value={manualHash}
                  onChange={(e) => setManualHash(e.target.value)}
                  placeholder="64 hex"
                  required
                />
              </div>
              <div className="form-group">
                <label>Public Key X25519 (64 hex)</label>
                <textarea
                  value={manualX25519}
                  onChange={(e) => setManualX25519(e.target.value)}
                  placeholder="64 hex"
                  required
                  rows={2}
                />
              </div>
            </>
          )}

          {error && <div className="error-message">{error}</div>}

          <div className="modal-actions">
            <button type="button" onClick={onClose} disabled={loading}>
              Cancel
            </button>
            <button
              type="submit"
              disabled={loading || (mode === "auto" && !parsed)}
            >
              {loading ? "..." : "Добавить"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

// ---------- Message Status Component ----------
const MessageStatus = ({
  timestamp,
  status,
}: {
  timestamp: Date;
  status: "sent" | "delivered" | "read";
}) => (
  <span className="status-container">
    <span className="time">
      {timestamp.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
    </span>
    <span className="check-wrapper">
      <svg
        className={`check-icon first ${status === "sent" ? "sent" : ""}`}
        viewBox="0 0 16 16"
        fill="none"
        width="14"
        height="14"
      >
        <path
          d="M13.5 4.5L6.5 11.5L2.5 7.5"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      {(status === "delivered" || status === "read") && (
        <svg
          className={`check-icon second ${status === "read" ? "read" : ""}`}
          viewBox="0 0 16 16"
          fill="none"
          width="14"
          height="14"
        >
          <path
            d="M13.5 4.5L6.5 11.5L2.5 7.5"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )}
    </span>
  </span>
);

export default App;
