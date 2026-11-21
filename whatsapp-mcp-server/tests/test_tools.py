import responses

import main
import whatsapp

BASE = whatsapp.WHATSAPP_API_BASE_URL


@responses.activate
def test_send_message_tool_forwards_reply_to_and_wraps_the_result():
    responses.post(f"{BASE}/send", json={"success": True, "message": "sent"})
    result = main.send_message("1@s.whatsapp.net", "hi", reply_to="ABC")
    assert result == {"success": True, "message": "sent"}
    assert responses.calls[0].request.body == b'{"recipient": "1@s.whatsapp.net", "message": "hi", "reply_to": "ABC"}'


def test_send_message_tool_requires_a_recipient():
    assert main.send_message("", "hi") == {"success": False, "message": "Recipient must be provided"}


@responses.activate
def test_delete_message_tool_wraps_the_result():
    responses.post(f"{BASE}/delete", json={"success": True, "message": "deleted"})
    assert main.delete_message("ABC", "1@s.whatsapp.net", for_me=True) == {"success": True, "message": "deleted"}


def test_delete_message_tool_requires_both_ids():
    assert main.delete_message("", "1@s.whatsapp.net")["message"] == "Message ID must be provided"
    assert main.delete_message("ABC", "")["message"] == "Chat JID must be provided"


@responses.activate
def test_edit_create_and_group_tools_wrap_the_result():
    responses.post(f"{BASE}/edit", json={"success": True, "message": "ABC"})
    responses.post(f"{BASE}/group/create", json={"success": True, "message": "120363000000000001@g.us"})
    responses.post(f"{BASE}/group/participants", json={"success": True, "message": "add"})
    assert main.edit_message("1@s.whatsapp.net", "ABC", "x") == {"success": True, "message": "ABC"}
    assert main.create_group("Team", []) == {"success": True, "message": "120363000000000001@g.us"}
    assert main.update_group_participants("120363000000000001@g.us", [], "add") == {"success": True, "message": "add"}


@responses.activate
def test_download_media_tool_reports_the_file_path():
    responses.post(f"{BASE}/download", json={"success": True, "path": "/tmp/a.jpg"})
    assert main.download_media("ABC", "1@s.whatsapp.net") == {
        "success": True,
        "message": "Media downloaded successfully",
        "file_path": "/tmp/a.jpg",
    }
