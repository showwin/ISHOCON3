# frozen_string_literal: true

class Entry < ActiveRecord::Base
  attribute :id, :string
  attribute :reservation_id, :string
  attribute :entry_at, :datetime
end
