defmodule PhxTestServerWeb.BinaryChannel do
  use Phoenix.Channel

  @impl true
  def join("binary:" <> _topic, _payload, socket) do
    {:ok, socket}
  end

  @impl true
  def handle_in("binary_data", {:binary, data}, socket) do
    # Process binary data and respond with processed binary
    processed_data = for <<byte::binary-size(1) <- data>>, into: <<>>, do: <<byte::binary, 100>>
    {:reply, {:ok, {:binary, processed_data}}, socket}
  end

  @impl true
  def handle_in("large_binary", {:binary, _data}, socket) do
    # Return a large binary payload
    large_data = :crypto.strong_rand_bytes(1024)
    {:reply, {:ok, {:binary, large_data}}, socket}
  end

  @impl true
  def handle_in("binary_broadcast", {:binary, data}, socket) do
    # Broadcast binary data to all subscribers
    broadcast!(socket, "binary_message", {:binary, data})
    {:reply, {:ok, %{broadcast_sent: true}}, socket}
  end
end