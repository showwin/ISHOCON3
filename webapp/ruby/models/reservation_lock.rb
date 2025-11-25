# frozen_string_literal: true

class ReservationLock < ActiveRecord::Base
  attribute :schedule_id, :string
  attribute :created_at, :datetime
end
