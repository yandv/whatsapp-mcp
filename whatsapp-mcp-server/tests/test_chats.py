from datetime import datetime, timezone

import pytest

import whatsapp
from conftest import ALICE, BOB, TEAM

pytestmark = pytest.mark.usefixtures("db_path")


class TestListChats:
    def test_orders_by_last_activity_with_the_last_message(self):
        chats = whatsapp.list_chats()
        assert [c.jid for c in chats] == [TEAM, BOB, ALICE]
        assert chats[2].last_message == "See you SOON"
        assert chats[2].last_sender == "5500000000001"
        assert chats[2].last_message_time == datetime(2026, 1, 1, 10, 4, tzinfo=timezone.utc)

    def test_sorts_by_name(self):
        assert [c.name for c in whatsapp.list_chats(sort_by="name")] == ["Alice", "Bob", "Team"]

    def test_can_skip_the_last_message(self):
        assert whatsapp.list_chats(include_last_message=False)[0].last_message is None

    @pytest.mark.parametrize("query, expected", [("ali", [ALICE]), ("TEAM", [TEAM]), ("0002", [BOB]), ("zzz", [])])
    def test_filters_by_name_or_jid_ignoring_case(self, query, expected):
        assert [c.jid for c in whatsapp.list_chats(query=query)] == expected

    def test_paginates(self):
        assert [c.jid for c in whatsapp.list_chats(limit=2, page=0)] == [TEAM, BOB]
        assert [c.jid for c in whatsapp.list_chats(limit=2, page=1)] == [ALICE]
        assert whatsapp.list_chats(limit=2, page=2) == []

    def test_group_detection_uses_the_jid_server(self):
        by_jid = {c.jid: c for c in whatsapp.list_chats()}
        assert by_jid[TEAM].is_group and not by_jid[ALICE].is_group


class TestSearchContacts:
    def test_matches_by_name_ignoring_case(self):
        contacts = whatsapp.search_contacts("alice")
        assert [(c.phone_number, c.name, c.jid) for c in contacts] == [("5500000000001", "Alice", ALICE)]

    def test_matches_by_phone_fragment(self):
        assert [c.name for c in whatsapp.search_contacts("0002")] == ["Bob"]

    def test_never_returns_groups(self):
        assert whatsapp.search_contacts("Team") == []
        assert whatsapp.search_contacts("120363") == []

    def test_orders_by_name(self):
        assert [c.name for c in whatsapp.search_contacts("5500000000")] == ["Alice", "Bob"]


class TestGetChat:
    def test_returns_the_chat_with_its_last_message(self):
        chat = whatsapp.get_chat(BOB)
        assert (chat.name, chat.last_message, chat.last_is_from_me) == ("Bob", "dinner tonight?", 0)

    def test_omits_the_last_message_when_asked(self):
        assert whatsapp.get_chat(BOB, include_last_message=False).last_message is None

    def test_unknown_jid_gives_none(self):
        assert whatsapp.get_chat("nobody@s.whatsapp.net") is None


class TestDirectChatByContact:
    def test_finds_the_direct_chat_by_phone_number(self):
        assert whatsapp.get_direct_chat_by_contact("5500000000002").jid == BOB

    def test_ignores_groups(self):
        assert whatsapp.get_direct_chat_by_contact("120363000000000001") is None


class TestContactChats:
    def test_includes_direct_chat_and_groups_the_contact_wrote_in(self):
        jids = {c.jid for c in whatsapp.get_contact_chats("5500000000001")}
        assert jids == {ALICE, TEAM}

    def test_paginates(self):
        assert len(whatsapp.get_contact_chats("5500000000001", limit=1, page=0)) == 1
        assert whatsapp.get_contact_chats("5500000000001", limit=1, page=5) == []
