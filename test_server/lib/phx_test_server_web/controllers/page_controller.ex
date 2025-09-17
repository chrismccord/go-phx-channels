defmodule PhxTestServerWeb.PageController do
  use PhxTestServerWeb, :controller

  def home(conn, _params) do
    render(conn, :home)
  end
end
