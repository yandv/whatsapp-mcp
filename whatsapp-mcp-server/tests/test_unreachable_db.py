import pytest

import whatsapp

pytestmark = pytest.mark.usefixtures("missing_db")


def test_listing_messages_returns_an_empty_list():
    assert whatsapp.list_messages() == []


def test_lookups_degrade_to_empty_values():
    assert whatsapp.list_chats() == []
    assert whatsapp.search_contacts("a") == []
    assert whatsapp.get_chat("1@s.whatsapp.net") is None
    assert whatsapp.get_last_interaction("1@s.whatsapp.net") is None
    assert whatsapp.get_sender_name("1@s.whatsapp.net") == "1@s.whatsapp.net"


def test_message_context_re_raises_the_database_error():
    import sqlite3

    with pytest.raises(sqlite3.Error):
        whatsapp.get_message_context("A1")
