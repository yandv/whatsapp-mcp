import re

import pytest

import whatsapp
from conftest import ALICE, BOB, TEAM

pytestmark = pytest.mark.usefixtures("db_path")


def contents(output: str) -> list[str]:
    """Strip the timestamp, chat and sender prefix from each printed line."""
    return [re.sub(r"^\[[^\]]*\] (Chat: .*? )?From: [^:]*: ", "", line) for line in output.splitlines()]


def listed(**kwargs) -> list[str]:
    kwargs.setdefault("include_context", False)
    return contents(whatsapp.list_messages(**kwargs))


class TestListMessages:
    def test_orders_newest_first_and_marks_own_messages(self):
        output = whatsapp.list_messages(chat_jid=ALICE, include_context=False, limit=2)
        assert output.splitlines() == [
            "[2026-01-01 10:04:00] Chat: Alice From: Alice: See you SOON",
            "[2026-01-01 10:03:00] Chat: Alice From: Me: nice one",
        ]

    def test_shows_media_id_and_chat_for_media_messages(self):
        output = whatsapp.list_messages(query="look at", include_context=False)
        assert output == (
            f"[2026-01-01 10:02:00] Chat: Alice From: Alice: "
            f"[image - Message ID: A3 - Chat JID: {ALICE}] look at this\n"
        )

    @pytest.mark.parametrize(
        "filters, expected",
        [
            ({"chat_jid": BOB}, ["dinner tonight?"]),
            ({"chat_jid": TEAM}, ["standup in 5"]),
            ({"sender_phone_number": "5500000000099"}, ["nice one", "hi Alice"]),
            ({"query": "soon"}, ["See you SOON"]),
            ({"query": "nothing matches this"}, ["No messages to display."]),
            ({"after": "2026-01-01T10:03:30", "before": "2026-01-02T12:00:00"}, ["dinner tonight?", "See you SOON"]),
        ],
    )
    def test_filters(self, filters, expected):
        assert listed(**filters) == expected

    def test_pages_through_results_without_overlap(self):
        first = listed(chat_jid=ALICE, limit=2, page=0)
        second = listed(chat_jid=ALICE, limit=2, page=1)
        third = listed(chat_jid=ALICE, limit=2, page=2)
        fourth = listed(chat_jid=ALICE, limit=2, page=3)
        assert first == ["See you SOON", "nice one"]
        assert second == [f"[image - Message ID: A3 - Chat JID: {ALICE}] look at this", "hi Alice"]
        assert third == ["hello there"]
        assert fourth == ["No messages to display."]

    def test_includes_neighbours_when_context_is_requested(self):
        output = whatsapp.list_messages(query="look at", context_before=1, context_after=1)
        assert contents(output) == [
            "hi Alice",
            f"[image - Message ID: A3 - Chat JID: {ALICE}] look at this",
            "nice one",
        ]

    def test_empty_result_prints_a_placeholder(self):
        assert whatsapp.list_messages(query="zzz", include_context=False) == "No messages to display."

    @pytest.mark.parametrize("field", ["after", "before"])
    def test_rejects_a_malformed_date(self, field):
        with pytest.raises(ValueError, match=f"Invalid date format for '{field}'"):
            whatsapp.list_messages(**{field: "yesterday"})


class TestMessageContext:
    def test_returns_messages_before_and_after_in_chat_order(self):
        context = whatsapp.get_message_context("A3", before=2, after=2)
        assert context.message.content == "look at this"
        assert [m.id for m in context.before] == ["A2", "A1"]
        assert [m.id for m in context.after] == ["A4", "A5"]

    def test_does_not_leak_other_chats(self):
        context = whatsapp.get_message_context("B1", before=5, after=5)
        assert context.before == [] and context.after == []

    def test_respects_the_requested_size(self):
        context = whatsapp.get_message_context("A3", before=1, after=0)
        assert [m.id for m in context.before] == ["A2"]
        assert context.after == []

    def test_unknown_message_raises(self):
        with pytest.raises(ValueError, match="Message with ID NOPE not found"):
            whatsapp.get_message_context("NOPE")


class TestLastInteraction:
    def test_formats_the_latest_message_of_the_contact(self):
        assert whatsapp.get_last_interaction(ALICE) == "[2026-01-01 10:04:00] Chat: Alice From: Alice: See you SOON\n"

    def test_returns_none_for_an_unknown_contact(self):
        assert whatsapp.get_last_interaction("9999@s.whatsapp.net") is None


class TestSenderName:
    def test_resolves_a_bare_phone_number_through_the_chat_name(self):
        assert whatsapp.get_sender_name("5500000000002") == "Bob"

    def test_resolves_a_full_jid(self):
        assert whatsapp.get_sender_name(BOB) == "Bob"

    def test_falls_back_to_the_raw_sender(self):
        assert whatsapp.get_sender_name("123456") == "123456"
