import re
import sqlite3
from pathlib import Path

import pytest

import whatsapp

BRIDGE_MAIN = Path(__file__).resolve().parents[2] / "whatsapp-bridge" / "main.go"


def bridge_schema() -> str:
    """Take the CREATE TABLE statements from the bridge, so the tests follow the real schema."""
    source = BRIDGE_MAIN.read_text()
    match = re.search(r"db\.Exec\(`(.*?CREATE TABLE IF NOT EXISTS messages.*?)`\)", source, re.S)
    assert match, "schema not found in whatsapp-bridge/main.go"
    return match.group(1)


ALICE = "5500000000001@s.whatsapp.net"
BOB = "5500000000002@s.whatsapp.net"
TEAM = "120363000000000001@g.us"


@pytest.fixture
def db_path(tmp_path, monkeypatch):
    path = tmp_path / "messages.db"
    conn = sqlite3.connect(path)
    conn.executescript(bridge_schema())
    chats = [
        (ALICE, "Alice", "2026-01-01 10:04:00+00:00"),
        (BOB, "Bob", "2026-01-02 09:00:00+00:00"),
        (TEAM, "Team", "2026-01-03 08:00:00+00:00"),
    ]
    conn.executemany("INSERT INTO chats VALUES (?, ?, ?)", chats)
    messages = [
        # id, chat, sender, content, timestamp, is_from_me, media_type, filename
        ("A1", ALICE, "5500000000001", "hello there", "2026-01-01 10:00:00+00:00", 0, None, None),
        ("A2", ALICE, "5500000000099", "hi Alice", "2026-01-01 10:01:00+00:00", 1, None, None),
        ("A3", ALICE, "5500000000001", "look at this", "2026-01-01 10:02:00+00:00", 0, "image", "photo.jpg"),
        ("A4", ALICE, "5500000000099", "nice one", "2026-01-01 10:03:00+00:00", 1, None, None),
        ("A5", ALICE, "5500000000001", "See you SOON", "2026-01-01 10:04:00+00:00", 0, None, None),
        ("B1", BOB, "5500000000002", "dinner tonight?", "2026-01-02 09:00:00+00:00", 0, None, None),
        ("T1", TEAM, "5500000000001", "standup in 5", "2026-01-03 08:00:00+00:00", 0, None, None),
    ]
    conn.executemany(
        "INSERT INTO messages (id, chat_jid, sender, content, timestamp, is_from_me, media_type, filename)"
        " VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
        messages,
    )
    conn.commit()
    conn.close()
    monkeypatch.setattr(whatsapp, "MESSAGES_DB_PATH", str(path))
    return path


@pytest.fixture
def missing_db(tmp_path, monkeypatch):
    monkeypatch.setattr(whatsapp, "MESSAGES_DB_PATH", str(tmp_path / "nope" / "messages.db"))
