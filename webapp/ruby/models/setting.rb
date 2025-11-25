# frozen_string_literal: true

class Setting < ActiveRecord::Base
  self.implicit_order_column = :initialized_at

  attribute :initialized_at, :datetime
end
