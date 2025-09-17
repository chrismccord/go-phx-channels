defmodule PhxTestServerWeb.RoomChannel do
  use Phoenix.Channel

  @impl true
  def join("room:" <> room_id, _payload, socket) do
    send(self(), {:after_join, room_id})
    {:ok, assign(socket, :room_id, room_id)}
  end

  @impl true
  def handle_info({:after_join, room_id}, socket) do
    push(socket, "joined", %{room_id: room_id})
    {:noreply, socket}
  end

  @impl true
  def handle_in("ping", payload, socket) do
    {:reply, {:ok, %{pong: payload}}, socket}
  end

  @impl true
  def handle_in("echo", payload, socket) do
    push(socket, "echo_reply", payload)
    {:reply, {:ok, %{echoed: true}}, socket}
  end

  @impl true
  def handle_in("broadcast", payload, socket) do
    broadcast!(socket, "broadcast_message", payload)
    {:reply, {:ok, %{broadcasted: true}}, socket}
  end

  @impl true
  def handle_in("error_test", _payload, _socket) do
    raise "Test error"
  end

  @impl true
  def handle_in("timeout_test", _payload, socket) do
    # Simulate a long operation by not replying
    {:noreply, socket}
  end

  @impl true
  def handle_in("binary_echo", {:binary, data}, socket) do
    # Echo the binary data back
    {:reply, {:ok, {:binary, data}}, socket}
  end

  @impl true
  def handle_in("push_binary", _payload, socket) do
    # Push some binary data
    push(socket, "binary_data", {:binary, <<1, 2, 3, 4>>})
    {:reply, {:ok, %{binary_pushed: true}}, socket}
  end
end