# frozen_string_literal: true

class ReservationSeat < ActiveRecord::Base
  attribute :id, :integer
  attribute :reservation_id, :string
  attribute :seat, :string
  attribute :created_at, :datetime
end
