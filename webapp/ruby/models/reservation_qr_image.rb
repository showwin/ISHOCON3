# frozen_string_literal: true

class ReservationQrImage < ActiveRecord::Base
  attribute :id, :string
  attribute :reservation_id, :string
  attribute :image, :binary
end
