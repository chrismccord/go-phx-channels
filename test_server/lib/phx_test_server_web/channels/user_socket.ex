defmodule PhxTestServerWeb.UserSocket do
  use Phoenix.Socket

  # Allow all origins for testing
  @impl true
  def connect(_params, socket, _connect_info) do
    {:ok, socket}
  end

  # Socket IDs are topics that allow you to identify all sockets for a given user:
  #
  #     def id(socket), do: "user_socket:#{socket.assigns.user_id}"
  #
  # Would allow you to broadcast a "disconnect" event and terminate
  # all active sockets and channels for a given user:
  #
  #     Elixir.PhxTestServerWeb.Endpoint.broadcast("user_socket:#{user.id}", "disconnect", %{})
  #
  # Returning `nil` makes this socket anonymous.
  @impl true
  def id(_socket), do: nil

  # Define channel routes
  channel "room:*", PhxTestServerWeb.RoomChannel
  channel "binary:*", PhxTestServerWeb.BinaryChannel
end