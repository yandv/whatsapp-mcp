import json

import pytest
import requests
import responses

import whatsapp

BASE = whatsapp.WHATSAPP_API_BASE_URL


def last_body() -> dict:
    return json.loads(responses.calls[-1].request.body)


@pytest.fixture(autouse=True)
def bridge():
    """Fake bridge at the network edge: nothing here ever reaches a real port."""
    responses.start()
    yield responses
    responses.stop()
    responses.reset()


class TestSendMessage:
    def test_posts_recipient_message_and_reply_to(self, bridge):
        bridge.post(f"{BASE}/send", json={"success": True, "message": "Message sent to 1 (id X)"})
        assert whatsapp.send_message("1@s.whatsapp.net", "hi", reply_to="ABC") == (True, "Message sent to 1 (id X)")
        assert last_body() == {"recipient": "1@s.whatsapp.net", "message": "hi", "reply_to": "ABC"}

    def test_reply_to_is_null_when_not_replying(self, bridge):
        bridge.post(f"{BASE}/send", json={"success": True, "message": "ok"})
        whatsapp.send_message("1", "hi")
        assert last_body()["reply_to"] is None

    def test_refuses_an_empty_recipient_without_calling_the_bridge(self):
        assert whatsapp.send_message("", "hi") == (False, "Recipient must be provided")

    def test_reports_a_failure_flag_from_the_bridge(self, bridge):
        bridge.post(f"{BASE}/send", json={"success": False, "message": "Not connected to WhatsApp"})
        assert whatsapp.send_message("1", "hi") == (False, "Not connected to WhatsApp")

    def test_reports_http_errors_with_status_and_body(self, bridge):
        bridge.post(f"{BASE}/send", body="Recipient is required", status=400)
        assert whatsapp.send_message("1", "hi") == (False, "Error: HTTP 400 - Recipient is required")

    def test_reports_network_failures(self, bridge):
        bridge.post(f"{BASE}/send", body=requests.ConnectionError("refused"))
        success, message = whatsapp.send_message("1", "hi")
        assert not success and message.startswith("Request error:")

    def test_reports_a_non_json_success_body_as_a_request_error(self, bridge):
        bridge.post(f"{BASE}/send", body="<html>", status=200)
        success, message = whatsapp.send_message("1", "hi")
        assert not success and message.startswith("Request error:")


class TestDeleteMessage:
    def test_revokes_for_everyone_by_default(self, bridge):
        bridge.post(f"{BASE}/delete", json={"success": True, "message": "deleted"})
        assert whatsapp.delete_message("ABC", "1@s.whatsapp.net") == (True, "deleted")
        assert last_body() == {"message_id": "ABC", "chat_jid": "1@s.whatsapp.net", "for_me": False}

    def test_can_delete_only_for_me(self, bridge):
        bridge.post(f"{BASE}/delete", json={"success": True, "message": "deleted"})
        whatsapp.delete_message("ABC", "1@s.whatsapp.net", for_me=True)
        assert last_body()["for_me"] is True

    @pytest.mark.parametrize(
        "message_id, chat_jid, expected",
        [("", "1@s.whatsapp.net", "Message ID must be provided"), ("ABC", "", "Chat JID must be provided")],
    )
    def test_requires_both_ids(self, message_id, chat_jid, expected):
        assert whatsapp.delete_message(message_id, chat_jid) == (False, expected)

    def test_reports_server_errors(self, bridge):
        bridge.post(f"{BASE}/delete", body="boom", status=500)
        assert whatsapp.delete_message("ABC", "1@s.whatsapp.net") == (False, "Error: HTTP 500 - boom")


class TestEditMessage:
    def test_posts_chat_message_id_and_new_text(self, bridge):
        bridge.post(f"{BASE}/edit", json={"success": True, "message": "ABC"})
        assert whatsapp.edit_message("1@s.whatsapp.net", "ABC", "fixed") == (True, "ABC")
        assert last_body() == {"chat_jid": "1@s.whatsapp.net", "message_id": "ABC", "message": "fixed"}

    def test_surfaces_the_bridge_error_message_on_500(self, bridge):
        bridge.post(f"{BASE}/edit", json={"success": False, "message": "edit window closed"}, status=500)
        assert whatsapp.edit_message("1@s.whatsapp.net", "ABC", "x") == (False, "edit window closed")

    def test_plain_text_errors_become_http_errors(self, bridge):
        bridge.post(f"{BASE}/edit", body="Invalid request format", status=400)
        assert whatsapp.edit_message("bad", "ABC", "x") == (False, "Error: HTTP 400 - Invalid request format")


class TestGroups:
    def test_create_group_posts_name_and_participants(self, bridge):
        bridge.post(f"{BASE}/group/create", json={"success": True, "message": "120363000000000001@g.us"})
        assert whatsapp.create_group("Team", ["1@s.whatsapp.net"]) == (True, "120363000000000001@g.us")
        assert last_body() == {"name": "Team", "participants": ["1@s.whatsapp.net"]}

    @pytest.mark.parametrize("action", ["add", "remove", "promote", "demote"])
    def test_update_participants_sends_the_action(self, bridge, action):
        bridge.post(f"{BASE}/group/participants", json={"success": True, "message": action})
        assert whatsapp.update_group_participants("120363000000000001@g.us", ["1@s.whatsapp.net"], action) == (True, action)
        assert last_body() == {"group": "120363000000000001@g.us", "participants": ["1@s.whatsapp.net"], "action": action}

    def test_group_failures_come_back_as_flags(self, bridge):
        bridge.post(f"{BASE}/group/create", json={"success": False, "message": "not allowed"}, status=500)
        assert whatsapp.create_group("Team", []) == (False, "not allowed")


class TestFiles:
    def test_send_file_posts_the_media_path(self, bridge, tmp_path):
        media = tmp_path / "a.png"
        media.write_bytes(b"x")
        bridge.post(f"{BASE}/send", json={"success": True, "message": "ok"})
        assert whatsapp.send_file("1", str(media)) == (True, "ok")
        assert last_body() == {"recipient": "1", "media_path": str(media)}

    def test_send_file_refuses_a_missing_file(self, tmp_path):
        assert whatsapp.send_file("1", str(tmp_path / "nope.png")) == (False, f"Media file not found: {tmp_path / 'nope.png'}")

    def test_send_audio_posts_an_ogg_as_is(self, bridge, tmp_path):
        media = tmp_path / "a.ogg"
        media.write_bytes(b"x")
        bridge.post(f"{BASE}/send", json={"success": True, "message": "ok"})
        assert whatsapp.send_audio_message("1", str(media)) == (True, "ok")
        assert last_body()["media_path"] == str(media)

    def test_download_media_returns_the_local_path(self, bridge):
        bridge.post(f"{BASE}/download", json={"success": True, "path": "/tmp/media/a.jpg"})
        assert whatsapp.download_media("ABC", "1@s.whatsapp.net") == "/tmp/media/a.jpg"
        assert last_body() == {"message_id": "ABC", "chat_jid": "1@s.whatsapp.net"}

    def test_download_media_gives_none_on_failure(self, bridge):
        bridge.post(f"{BASE}/download", json={"success": False, "message": "no media"})
        assert whatsapp.download_media("ABC", "1@s.whatsapp.net") is None
