defmodule PhxTestServerWeb.BinarySocket do
  use Phoenix.Socket

  # Allow all origins for testing
  @impl true
  def connect(_params, socket, _connect_info) do
    {:ok, socket}
  end

  @impl true
  def id(_socket), do: nil

  # Define channel routes
  channel "binary:*", PhxTestServerWeb.BinaryChannel
end